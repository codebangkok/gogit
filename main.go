package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func main() {
	var fDir = flag.String("dir", ".", "git directory")
	var fTree = flag.Bool("tree", false, "show tree")
	var fBlob = flag.Bool("blob", false, "show blob")
	var fBranch = flag.Bool("branch", false, "show branch")
	var fHead = flag.Bool("head", false, "show head")
	var fHistory = flag.Bool("history", false, "show commit history")
	var fContent = flag.Bool("content", false, "show blob content")
	var fIndex = flag.Bool("index", false, "show index,staging,cached")
	var fWatch = flag.Bool("watch", false, "watching repo change")
	flag.Parse()

	gogit(fDir, fTree, fBlob, fBranch, fHead, fHistory, fContent, fIndex)

	if !*fWatch {
		return
	}

	//==================================
	// File Watcher
	//==================================
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal("NewWatcher failed: ", err)
	}
	defer watcher.Close()

	fmt.Printf("watching..%v\n", *fDir)

	done := make(chan bool)
	go func() {
		defer close(done)

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				if event.Op == fsnotify.Create && event.Name[len(event.Name)-5:] != ".lock" {
					log.Printf("%s\n", event.Name)
					gogit(fDir, fTree, fBlob, fBranch, fHead, fHistory, fContent, fIndex)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}

	}()

	err = watcher.Add(fmt.Sprintf("%v/.git/refs/heads", *fDir))
	if err != nil {
		fmt.Println("no git repository")
		return
	}
	err = watcher.Add(fmt.Sprintf("%v/.git", *fDir))
	if err != nil {
		fmt.Println("no git repository")
		return
	}
	<-done
}

func gogit(fDir *string, fTree *bool, fBlob *bool, fBranch *bool, fHead *bool, fHistory *bool, fContent *bool, fIndex *bool) {

	//==================================
	// Markdown Mermaid
	//==================================
	markdown, err := os.Create("diagram.md")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		markdown.WriteString("```\n")
		markdown.Close()
	}()

	markdown.WriteString("```mermaid\n")
	markdown.WriteString("graph LR\n")

	//==================================
	// GIT
	//==================================
	repo, err := git.PlainOpen(*fDir)
	if err != nil {
		markdown.WriteString("empty((empty))\n")
		return
	}

	commiter, err := repo.CommitObjects()
	if err != nil {
		markdown.WriteString("empty((empty))\n")
		return
	}
	defer commiter.Close()

	// Index
	if *fIndex {

		index, err := repo.Storer.Index()
		if err != nil {
			markdown.WriteString("empty((empty))\n")
			return
		}

		if len(index.Entries) <= 0 {
			markdown.WriteString("Index[(index)]\n")
			return
		}

		if index.Cache == nil || len(index.Cache.Entries) <= 0 {
			markdown.WriteString("style Index fill:#e3f542,stroke:#333,color:#000000\n")
		}

		if *fBlob {
			for _, indexEntry := range index.Entries {

				if indexEntry.Name == ".gitignore" {
					continue
				}

				indexEntryHash := indexEntry.Hash.String()[:4]

				if *fContent {
					blob, err := repo.BlobObject(indexEntry.Hash)
					if err != nil {
						log.Fatal(err)
						return
					}

					r, err := blob.Reader()
					if err != nil {
						log.Fatal(err)
						return
					}
					content := new(strings.Builder)
					_, err = io.Copy(content, r)
					if err != nil {
						log.Fatal(err)
						return
					}
					markdown.WriteString(fmt.Sprintf("Index[(index)]--%v-->%v[%v %v]\n", indexEntry.Name, indexEntryHash, indexEntryHash, content))
				} else {
					markdown.WriteString(fmt.Sprintf("Index[(index)]--%v-->%v[%v]\n", indexEntry.Name, indexEntryHash, indexEntryHash))
				}
			}
		} else {
			markdown.WriteString("Index[(index)]\n")
		}
	} else {
		_, err := repo.Head()
		if err != nil {
			markdown.WriteString("empty((empty))\n")
			return
		}
	}

	err = commiter.ForEach(func(c *object.Commit) error {

		commitHash := c.Hash.String()[:4]
		treeHash := c.TreeHash.String()[:4]
		tree, err := c.Tree()
		if err != nil {
			return err
		}
		// Tree and Blob
		if *fTree {
			markdown.WriteString(fmt.Sprintf("%v(((%v)))-->%v{%v}\n", commitHash, commitHash, treeHash, treeHash))

			if *fBlob {
				return listTreeEntries(tree, markdown, fContent)
			}
		} else {
			if *fBlob {
				tree.Files().ForEach(func(f *object.File) error {
					if f.Name != ".gitignore" {
						blobHash := f.Hash.String()[:4]
						if *fContent {
							content, err := f.Contents()
							if err != nil {
								return err
							}
							markdown.WriteString(fmt.Sprintf("%v(((%v)))--%v-->%v[%v %v]\n", commitHash, commitHash, f.Name, blobHash, blobHash, content))
						} else {
							markdown.WriteString(fmt.Sprintf("%v(((%v)))--%v-->%v[%v]\n", commitHash, commitHash, f.Name, blobHash, blobHash))
						}
					}
					return nil
				})
			} else {
				markdown.WriteString(fmt.Sprintf("%v(((%v)))\n", commitHash, commitHash))
			}
		}

		// Commit History
		if *fHistory {
			c.Parents().ForEach(func(c2 *object.Commit) error {
				markdown.WriteString(fmt.Sprintf("%v(((%v)))-->%v(((%v)))\n", commitHash, commitHash, c2.Hash.String()[:4], c2.Hash.String()[:4]))
				return nil
			})
		}

		return nil
	})

	if err != nil {
		log.Fatal(err)
		return
	}

	// Branch and Head
	if *fBranch {
		branches, err := repo.Branches()
		if err != nil {
			return
		}
		err = branches.ForEach(func(r *plumbing.Reference) error {
			markdown.WriteString(fmt.Sprintf("%v[[%v]]-->%v\n", r.Name().Short(), r.Name().Short(), r.Hash().String()[:4]))
			return nil
		})
		if err != nil {
			log.Fatal(err)
			return
		}
	}

	// HEAD
	if *fHead {
		head, err := repo.Head()
		if err != nil {
			if !*fIndex {
				markdown.WriteString("empty((empty))\n")
			}
			return
		}
		if head.Name().IsBranch() && *fBranch {
			markdown.WriteString(fmt.Sprintf("HEAD{{HEAD}}-->%v\n", head.Name().Short()))
		} else {
			markdown.WriteString(fmt.Sprintf("HEAD{{HEAD}}-->%v\n", head.Hash().String()[:4]))
		}
	}

}

func listTreeEntries(tree *object.Tree, markdown *os.File, fContent *bool) error {
	treeHash := tree.Hash.String()[:4]
	for _, entry := range tree.Entries {
		if entry.Mode.IsFile() {
			file, err := tree.TreeEntryFile(&entry)
			if err != nil {
				return err
			}

			if file.Name == ".gitignore" {
				continue
			}

			blobHash := file.Hash.String()[:4]

			if *fContent {
				content, err := file.Contents()
				if err != nil {
					return err
				}
				markdown.WriteString(fmt.Sprintf("%v--%v-->%v[%v %v]\n", treeHash, file.Name, blobHash, blobHash, content))
			} else {
				markdown.WriteString(fmt.Sprintf("%v--%v-->%v[%v]\n", treeHash, file.Name, blobHash, blobHash))
			}
		} else {
			subTree, err := tree.Tree(entry.Name)
			if err != nil {
				return err
			}
			treeHash := tree.Hash.String()[:4]
			subTreeHash := subTree.Hash.String()[:4]
			markdown.WriteString(fmt.Sprintf("%v{%v}--%v-->%v{%v}\n", treeHash, treeHash, entry.Name, subTreeHash, subTreeHash))
			listTreeEntries(subTree, markdown, fContent)
		}
	}
	return nil
}
