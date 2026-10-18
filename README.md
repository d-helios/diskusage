# diskusage

A personal tool for keeping an eye on what takes up space on my disk, and for
cleaning it up.

It scans a directory tree and shows the biggest space consumers in a terminal
UI. You can walk around the tree, filter entries, reveal things in Finder, and
delete whatever is no longer needed.

```
diskusage [flags] [path]
```

| flag | meaning |
| --- | --- |
| `-j N` | directories read in parallel (default: 4 × CPU cores) |
| `-trash` | move deleted items to the Trash instead of deleting them |
| `-cross` | descend into other filesystems |
| `-apparent` | use apparent file sizes instead of allocated blocks |
| `-version` | print the version |

Main keys: `↑↓` move, `→`/`enter` open, `←` back, `space` mark, `d` delete,
`/` filter, `s` sort, `?` help, `q` quit.


</details><summary>Screenshots</summary>

<img width="1330" height="746" alt="image" src="https://github.com/user-attachments/assets/9f535c24-9195-482d-a051-3957df39e176" />


</details>


## Build

```sh
go build ./cmd/diskusage
```

macOS and Linux are supported. On Linux, deleting moves items permanently;
`-trash` is macOS-only.

## Test

```sh
go test ./...
```
