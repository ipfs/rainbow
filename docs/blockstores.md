# Rainbow Blockstores

`rainbow` ships with a number of possible backing block storage options for the purposes of caching data locally.
Because `rainbow`, as a gateway-only IPFS implementation, is not designed for long-term data storage there are no long
term guarantees of support for any particular backing blockstore.

`rainbow` currently ships with the following blockstores:

- [FlatFS](#flatfs)
- [Badger](#badger)

Note: `rainbow` exposes minimal configurability of each blockstore, if in your experimentation you note that tuning some
parameters is a big benefit to you file an issue/PR to discuss changing the blockstores parameters or if there's demand
to expose more configurability.

## FlatFS

FlatFS is a fairly simple blockstore that puts each block into a separate file on disk. Due to the heavy usage of the
filesystem (i.e. not just how bytes are stored on disk but file and directory structure as well) there are various
optimizations to be had in selection of the filesystem and disk types. For example, choosing a filesystem that enables
putting file metadata on a fast SSD while keeping the actual data on a slower disk might ease various lookup types.

## Badger

`rainbow` ships with [Badger-v4](https://github.com/dgraph-io/badger).
The main reasons to choose Badger compared to FlatFS are:
- It uses far fewer file descriptors and disk operations
- It comes with the ability to compress data on disk
- Generally faster reads and writes
- Native bloom filters

The main difficulty with Badger is that its internal garbage collection functionality (not `rainbow`'s) is dependent on
workload which makes it difficult to ahead-of-time judge the kinds of capacity you need.