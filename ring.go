package ring

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

var pageSize = uintptr(os.Getpagesize())

type Ring struct {
	size, contentLen, tail uintptr
	buffer                 *byte // manually managed memory
}

func New(size uintptr) (*Ring, error) {
	// outlined so this can be inlined and &Ring{} heap allocation ellided
	r := &Ring{}
	if err := r.Init(size); err != nil {
		return nil, err
	}
	return r, nil
}

// Init initializes the ring buffer with the given size.
func (r *Ring) Init(size uintptr) (err error) {
	if r.buffer != nil {
		return fmt.Errorf("ring already initialized")
	}

	if pageSize&(pageSize-1) != 0 {
		return fmt.Errorf("page size must be a power of 2")
	}

	if size%pageSize != 0 {
		return fmt.Errorf("size must be a multiple of the page size")
	}

	file, err := unix.MemfdCreate("github.com/Jorropo/mmu-ring", unix.MFD_CLOEXEC) // name does nothing and just used for debug
	if err != nil {
		return fmt.Errorf("memfd_create: %w", err)
	}
	defer unix.Close(file) // linux will cleanup the files once the mappings are unmapped

	if err := unix.Ftruncate(file, int64(size)); err != nil {
		return fmt.Errorf("ftruncate: %w", err)
	}
	totalMappingSize := size * 2
	if totalMappingSize < size {
		return fmt.Errorf("size overflow when creating MMU-ring")
	}

	// temporary mapping to allocate twice the ring of virtual memory
	orig, err := unix.MmapPtr(-1, 0, nil, totalMappingSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_ANONYMOUS|unix.MAP_NORESERVE|unix.MAP_PRIVATE)
	if err != nil {
		return fmt.Errorf("virtual mmap: %w", err)
	}
	defer func() {
		if err != nil {
			unix.MunmapPtr(orig, totalMappingSize)
		}
	}()

	// replace the virtual reservation with the physical memory tail to head
	_, err = unix.MmapPtr(file, 0, orig, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_FIXED)
	if err != nil {
		return fmt.Errorf("first physical mmap: %w", err)
	}
	_, err = unix.MmapPtr(file, 0, unsafe.Add(orig, size), size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_FIXED)
	if err != nil {
		return fmt.Errorf("second physical mmap: %w", err)
	}

	*r = Ring{size: size, buffer: (*byte)(orig)}
	return nil
}

func (r *Ring) Close() error {
	if r.buffer == nil {
		return nil
	}
	err := unix.MunmapPtr(unsafe.Pointer(r.buffer), r.size*2)
	r.buffer = nil
	return err
}

func (r *Ring) Size() uintptr {
	return r.size
}

// Unused returns a single contiguous slice of unused memory in the ring so you can pass it to .Read or whatever is gonna write to it.
func (r *Ring) Unused() []byte {
	return unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(r.buffer), r.tail+r.contentLen)), int(r.freeSpace()))
}

// Advance bumps the head, in other words it tells the ring that you have written to the slice returned by .Unused.
func (r *Ring) Advance(n uintptr) error {
	if n > r.freeSpace() {
		return fmt.Errorf("not enough space in ring")
	}

	r.contentLen += n
	return nil
}

func (r *Ring) freeSpace() uintptr {
	return r.size - r.contentLen
}

// Write is an alternative to Unused and Advance, you get called back with a reference to the unused buffer and return how many new bytes are there.
func (r *Ring) Write(f func(buffer []byte) (newData uintptr, err error)) (newData uintptr, err error) {
	newData, err = f(r.Unused())
	if err != nil {
		return 0, err
	}
	err = r.Advance(newData)
	if err != nil {
		return 0, err
	}
	return newData, nil
}

func (r *Ring) Content() []byte {
	return unsafe.Slice((*byte)(unsafe.Add(unsafe.Pointer(r.buffer), r.tail)), int(r.contentLen))
}

func (r *Ring) Consume(n uintptr) error {
	if n > r.contentLen {
		return fmt.Errorf("not enough data in ring")
	}

	r.tail = (r.tail + n) % r.size
	r.contentLen -= n
	return nil
}

// Read is an alternative to Content and Consume, you get called back with a reference to the used buffer and return how many bytes you have consumed.
func (r *Ring) Read(f func(buffer []byte) (consumed uintptr, err error)) (consumed uintptr, err error) {
	consumed, err = f(r.Content())
	if err != nil {
		return 0, err
	}
	err = r.Consume(consumed)
	if err != nil {
		return 0, err
	}
	return consumed, nil
}
