package main

import (
	"context"
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"

	fsdiff "github.com/b5/wnfs-sync/fsdiff"
	"github.com/ipfs/go-cid"
	golog "github.com/ipfs/go-log"
	wnfs "github.com/qri-io/wnfs-go"
	wnipfs "github.com/qri-io/wnfs-go/ipfs"
	"github.com/qri-io/wnfs-go/mdstore"
	cli "github.com/urfave/cli/v2"
)

const linkFilename = ".wnfs-sync"

func open(ctx context.Context) (wnfs.WNFS, *ExternalState) {
	ipfsPath := os.Getenv("IPFS_PATH")
	if ipfsPath == "" {
		dir, err := configDirPath()
		if err != nil {
			errExit("error: getting configuration directory: %s\n", err)
		}
		ipfsPath = filepath.Join(dir, "ipfs")

		if _, err := os.Stat(filepath.Join(ipfsPath, "config")); os.IsNotExist(err) {
			if err := os.MkdirAll(ipfsPath, 0755); err != nil {
				errExit("error: creating ipfs repo: %s\n", err)
			}
			fmt.Printf("creating ipfs repo at %s ... ", ipfsPath)
			if err = wnipfs.InitRepo(ipfsPath, ""); err != nil {
				errExit("\nerror: %s", err)
			}
			fmt.Println("done")
		}
	}

	store, err := wnipfs.NewFilesystem(ctx, map[string]interface{}{
		"path": ipfsPath,
	})

	if err != nil {
		errExit("error: opening IPFS repo: %s\n", err)
	}

	statePath, err := ExternalStatePath()
	if err != nil {
		errExit("error: getting state path: %s\n", err)
	}
	state, err := LoadOrCreateExternalState(statePath)
	if err != nil {
		errExit("error: loading external state: %s\n", err)
	}

	var fs wnfs.WNFS
	if state.RootCID.Equals(cid.Cid{}) {
		fmt.Printf("creating new wnfs filesystem...")
		if fs, err = wnfs.NewEmptyFS(ctx, store); err != nil {
			errExit("error: creating empty WNFS: %s\n", err)
		}
		fmt.Println("done")
	} else {
		if fs, err = wnfs.FromCID(ctx, store, state.RootCID); err != nil {
			errExit("error: opening WNFS CID %s: %s\n", state.RootCID, err.Error())
		}
	}

	return fs, state
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		fsys                wnfs.WNFS
		state               *ExternalState
		updateExternalState func()
	)

	app := &cli.App{
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Usage:   "print verbose output",
			},
		},
		Description: "sync between local and web-native filesystems",
		Before: func(c *cli.Context) error {
			if c.Bool("verbose") {
				golog.SetLogLevel("wnfs", "debug")
			}

			fsys, state = open(ctx)
			updateExternalState = func() {
				state.RootCID = fsys.(mdstore.DagNode).Cid()
				fmt.Printf("writing root cid: %s...", state.RootCID)
				if err := state.Write(); err != nil {
					errExit("error: writing external state: %s\n", err)
				}
				fmt.Println("done")
			}

			return nil
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "initialze a wnfs-sync root",
				Action: func(c *cli.Context) error {
					pwd, err := os.Getwd()
					if err != nil {
						return err
					}

					projectName := filepath.Base(pwd)
					wnfsPath := filepath.Join("public", projectName)
					if _, lsErr := fsys.Ls(wnfsPath); lsErr == nil {
						return fmt.Errorf("project named %q already exists", projectName)
					}

					linkFilepath := filepath.Join(pwd, linkFilename)
					if err := ioutil.WriteFile(linkFilepath, []byte(wnfsPath), 0755); err != nil {
						return err
					}

					if err := fsys.Mkdir(wnfsPath, wnfs.MutationOptions{Commit: true}); err != nil {
						// rollback
						os.RemoveAll(linkFilepath)
						return err
					}

					updateExternalState()
					return nil
				},
			},
			{
				Name:  "status",
				Usage: "show differences between latest snapshot & filesystem",
				Action: func(c *cli.Context) error {
					deltas, err := fileDelta(fsys)
					if err != nil {
						return err
					}

					if deltas.Type == fsdiff.DTUnchanged {
						fmt.Println("nothing to commit, working tree clean")
						return nil
					}

					os.Stdout.Write([]byte(deltaTreeString(deltas)))
					return nil
				},
			},
			{
				Name:  "commit",
				Usage: "write filesystem snapshot to wnfs",
				Action: func(c *cli.Context) error {
					deltas, err := fileDelta(fsys)
					if err != nil {
						return err
					}

					if deltas.Type == fsdiff.DTUnchanged {
						fmt.Println("nothing to commit, working tree clean")
						return nil
					}

					pwd, err := os.Getwd()
					if err != nil {
						return err
					}
					linkFilepath := filepath.Join(pwd, linkFilename)
					wnfsPathBytes, err := ioutil.ReadFile(linkFilepath)
					if err != nil {
						return fmt.Errorf("%q is not a linked directory", pwd)
					}

					defer updateExternalState()
					return writeSnapshot(
						fsys,
						os.DirFS(filepath.Dir(pwd)),
						string(wnfsPathBytes),
						filepath.Base(pwd),
						deltas,
					)
				},
			},
			{
				Name:  "cat",
				Usage: "cat a file",
				Action: func(c *cli.Context) error {
					data, err := fsys.Cat(c.Args().Get(0))
					if err != nil {
						return err
					}
					_, err = os.Stdout.Write(data)
					return err
				},
			},
			{
				Name:  "ls",
				Usage: "list the contents of a directory",
				Action: func(c *cli.Context) error {
					entries, err := fsys.Ls(c.Args().Get(0))
					if err != nil {
						return err
					}

					for _, entry := range entries {
						fmt.Println(entry.Name())
					}
					return nil
				},
			},
			{
				Name:  "tree",
				Usage: "show a tree rooted at a given path",
				Action: func(c *cli.Context) error {
					path := c.Args().Get(0)
					// TODO(b5): can't yet create tree from wnfs root.
					// for now replace empty string with "public"
					if path == "" {
						path = "public"
					}

					s, err := treeString(fsys, path)
					if err != nil {
						return err
					}

					os.Stdout.Write([]byte(s))
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		errExit(err.Error())
	}
}

func errExit(msg string, v ...interface{}) {
	fmt.Printf(msg, v...)
	os.Exit(1)
}

func fileDelta(fsys wnfs.WNFS) (*fsdiff.Delta, error) {
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	linkFilepath := filepath.Join(pwd, linkFilename)

	wnfsPathBytes, err := ioutil.ReadFile(linkFilepath)
	if err != nil {
		return nil, fmt.Errorf("%q is not a linked directory", pwd)
	}

	return fsdiff.Tree(
		string(wnfsPathBytes),
		filepath.Base(pwd),
		fsys,
		os.DirFS(filepath.Dir(pwd)),
		".wnfs-sync",
	)
}

func writeSnapshot(fsys wnfs.WNFS, local fs.FS, wnfsPath, localPath string, delta *fsdiff.Delta) error {
	for _, d := range delta.Deltas {
		switch d.Type {
		case fsdiff.DTAdd, fsdiff.DTChange:
			// added or changed files
			if len(d.Deltas) == 0 {
				p := filepath.Join(localPath, d.Name)
				fmt.Printf("writing %s\n", p)
				f, err := local.Open(p)
				if err != nil {
					return err
				}
				if err := fsys.Write(filepath.Join(wnfsPath, d.Name), f, wnfs.MutationOptions{Commit: true}); err != nil {
					return err
				}
				continue
			}

			// recurse for modified or added directories
			err := writeSnapshot(
				fsys,
				local,
				filepath.Join(wnfsPath, d.Name),
				filepath.Join(localPath, d.Name),
				d,
			)
			if err != nil {
				return err
			}
		case fsdiff.DTRemove:
			if err := fsys.Rm(filepath.Join(wnfsPath, d.Name), wnfs.MutationOptions{Commit: true}); err != nil {
				return err
			}
		}
	}
	return nil
}
