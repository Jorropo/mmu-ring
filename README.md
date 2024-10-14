# MMU-ring

MMU-ring is a copy-free hardware ring buffer implementation.

It never copies data around, yet it always provide single contiguous buffers for both content and unused space.

This is achieved by using the CPU's MMU to map the same physical memory twice head to tail.
This means, in virtual memory we allocate a buffer twice as big, however the second half points to the same physical memory as the first half.
Then if the ring would wrap around, rather than having to handle this into two steps or return two buffers we can overflow from the first mapping into the second mirror mapping.

The buffers are valid for any operation, can be passed to the OS like with os.File.Read.

Creating and destroying an MMU buffer require doing a couple of syscalls, altho no IO and is slightly costier than a heap allocation. There is no ongoing CPU cost once it has been created.

You need to call `.Close` when you are done using the buffers, the special mappings are not collected by GC.

This is incompatible with linux no MMU (altho go does not support linux no MMU anyway so not sure how you would manage to run anyway).

# TODO:

- Windows support, they have APIs designed to do very exactly this.
- MacOS Support, idk.
- Generic posix support, shm ¿ altho that not anonymous.
- Copy-free Resizing (remap)