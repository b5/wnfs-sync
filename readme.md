# wnfs-sync

[demo](https://asciinema.org/a/hSmMWjkndZ58LPYAP86g0j3CQ)

demo CLI application to syncronize a local filesystem directory to a public [web-native file system](https://whitepaper.fission.codes/file-system/file-system-basics) directory.

```sh
# get this binary, assumes you have go & make installed
$ git clone https://github.com/b5/wnfs-sync.git
$ cd wnfs-sync && go mod tidy && make install

# set a place for wnfs to store the root hash of it's filesystem on IPFS
$ export WNFS_STATE_PATH="${HOME}/wnfs.json"
# optional: set a place to read ipfs from
$ export IPFS_PATH="${HOME}/.ipfs"

$ mkdir sync_example && cd sync_example
$ wnfs-sync init

# make a change:
$ echo "oh hai" > hai.txt

# this'll show a dirty local tree:
$ wnfs-sync status

# commit changes:
$ wnfs-sync commit

# more changes
$ echo "such change" > hai.txt
$ echo "such addition" > bye.txt
# commit
$ wnfs-sync commit

# this'll show no changes:
$ wnfs-sync status
# show contents of wnfs:
$ wnfs-sync tree

# read from wnfs:
$ wnfs-sync cat public/sync_example/hai.txt
# "such change"
```

### Development Status: Work-In-Progress
This repo is very much a work-in-progress, and doesn't yet produce proper WNFS data. Don't rely on this.