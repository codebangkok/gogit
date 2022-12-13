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
	var fContent = flag.Bool("content", false, "show blob content (one line max 10 char)")
	var fIndex = flag.Bool("index", false, "show index,staging,cached")
	var fRemote = flag.Bool("remote", false, "show remote")
	var fWatch = flag.Bool("watch", false, "watching repo change")
	flag.Parse()

	gogit(fDir, fTree, fBlob, fBranch, fHead, fHistory, fContent, fIndex, fRemote)

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

				isLock := len(event.Name) > 5 && event.Name[len(event.Name)-5:] == ".lock"
				isLogs := len(event.Name) > 4 && event.Name[len(event.Name)-4:] == "logs"
				isCommitEditMsg := len(event.Name) > 14 && event.Name[len(event.Name)-14:] == "COMMIT_EDITMSG"
				isOrigHead := len(event.Name) > 9 && event.Name[len(event.Name)-9:] == "ORIG_HEAD"

				if event.Op == fsnotify.Create && !isLock && !isLogs && !isCommitEditMsg && !isOrigHead {
					log.Printf("%s\n", event.Name)
					gogit(fDir, fTree, fBlob, fBranch, fHead, fHistory, fContent, fIndex, fRemote)
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

	repo, err := git.PlainOpen(*fDir)
	if err != nil {
		fmt.Println("no repository")
		return
	}

	remotes, err := repo.Remotes()
	for _, remote := range remotes {
		watcher.Add(fmt.Sprintf("%v/.git/refs/remotes/%v", *fDir, remote.Config().Name))
		if err != nil {
			fmt.Println("no remote repository")
			return
		}
	}

	<-done
}

func gogit(fDir *string, fTree *bool, fBlob *bool, fBranch *bool, fHead *bool, fHistory *bool, fContent *bool, fIndex *bool, fRemote *bool) {

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

	err = GenerateIndex(repo, markdown, fIndex, fBlob, fContent)
	if err != nil {
		log.Fatalf("GenerateIndex: %v", err)
	}

	err = commiter.ForEach(func(c *object.Commit) error {
		return GenerateTree(c, markdown, fTree, fBlob, fContent, fHistory)
	})

	if err != nil {
		log.Fatalf("GenerateTree: %v", err)
	}

	err = GenerateBranch(repo, markdown, fBranch)
	if err != nil {
		log.Fatalf("GenerateBranch: %v", err)
	}

	GenerateRemote(repo, markdown, fRemote, fTree, fBlob, fContent, fHistory)

	GenerateHead(repo, markdown, fHead, fIndex, fBranch)
	if err != nil {
		return
	}
}

func GenerateTree(commit *object.Commit, markdown *os.File, fTree *bool, fBlob *bool, fContent *bool, fHistory *bool) error {
	commitHash := commit.Hash.String()[:4]
	treeHash := commit.TreeHash.String()[:4]
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	// Tree and Blob
	if *fTree {
		markdown.WriteString(fmt.Sprintf("%v(((%v)))-->%v{%v}\n", commitHash, commitHash, treeHash, treeHash))

		if *fBlob {
			return GenerateTreeEntries(tree, markdown, fContent)
		}
	} else {
		if *fBlob {
			err = tree.Files().ForEach(func(f *object.File) error {
				if f.Name != ".gitignore" {
					blobHash := f.Hash.String()[:4]
					if *fContent {
						content, err := f.Contents()
						if err != nil {
							return err
						}
						contentText := TruncateString(content, 10)
						markdown.WriteString(fmt.Sprintf("%v(((%v)))--%v-->%v[%v %v]\n", commitHash, commitHash, f.Name, blobHash, blobHash, contentText))
					} else {
						markdown.WriteString(fmt.Sprintf("%v(((%v)))--%v-->%v[%v]\n", commitHash, commitHash, f.Name, blobHash, blobHash))
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		} else {
			markdown.WriteString(fmt.Sprintf("%v(((%v)))\n", commitHash, commitHash))
		}
	}

	// Commit History
	if *fHistory {
		err = commit.Parents().ForEach(func(c2 *object.Commit) error {
			markdown.WriteString(fmt.Sprintf("%v(((%v)))-.->%v(((%v)))\n", commitHash, commitHash, c2.Hash.String()[:4], c2.Hash.String()[:4]))
			return nil
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func GenerateIndex(repo *git.Repository, markdown *os.File, fIndex *bool, fBlob *bool, fContent *bool) error {
	// Index
	if *fIndex {
		index, err := repo.Storer.Index()
		if err != nil {
			markdown.WriteString("empty((empty))\n")
			return err
		}

		if len(index.Entries) <= 0 {
			markdown.WriteString("Index[(index)]\n")
			return err
		} else if len(index.Entries) == 1 && index.Entries[0].Name == ".gitignore" {
			markdown.WriteString("Index[(index)]\n")
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
						return err
					}

					r, err := blob.Reader()
					if err != nil {
						return err
					}
					content := new(strings.Builder)
					_, err = io.Copy(content, r)
					if err != nil {
						return err
					}

					contentText := TruncateString(content.String(), 10)
					markdown.WriteString(fmt.Sprintf("Index[(index)]--%v-->%v[%v %v]\n", indexEntry.Name, indexEntryHash, indexEntryHash, contentText))
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
			return err
		}
	}
	return nil
}

func GenerateBranch(repo *git.Repository, markdown *os.File, fBranch *bool) error {
	// Branch
	if *fBranch {
		branches, err := repo.Branches()
		if err != nil {
			return err
		}
		err = branches.ForEach(func(r *plumbing.Reference) error {
			markdown.WriteString(fmt.Sprintf("%v[[%v]]-->%v\n", r.Name().Short(), r.Name().Short(), r.Hash().String()[:4]))
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func GenerateHead(repo *git.Repository, markdown *os.File, fHead *bool, fIndex *bool, fBranch *bool) error {
	// HEAD
	if *fHead {
		head, err := repo.Head()
		if err != nil {
			if !*fIndex {
				markdown.WriteString("empty((empty))\n")
			}
			return err
		}
		markdown.WriteString("style HEAD fill:#266e38,stroke:#333,color:#ffffff\n")
		if head.Name().IsBranch() && *fBranch {
			markdown.WriteString(fmt.Sprintf("HEAD{{HEAD}}-->%v\n", head.Name().Short()))
		} else {
			markdown.WriteString(fmt.Sprintf("HEAD{{HEAD}}-->%v\n", head.Hash().String()[:4]))
		}
	}
	return nil
}

func GenerateRemote(repo *git.Repository, markdown *os.File, fRemote *bool, fTree *bool, fBlob *bool, fContent *bool, fHistory *bool) error {
	if *fRemote {

		remotes, err := repo.Remotes()
		if err != nil {
			return err
		}
		for _, remote := range remotes {
			markdown.WriteString(fmt.Sprintf("subgraph %v\n", remote.Config().Name))
			servers, err := remote.List(&git.ListOptions{})
			if err != nil {
				markdown.WriteString("end\n")
				markdown.WriteString(fmt.Sprintf("style %v fill:#1cb8e8,color:#000000\n", remote.Config().Name))
				return err
			}
			for _, server := range servers {
				if server.Name() == "HEAD" {
					continue
				}
				markdown.WriteString(fmt.Sprintf("%v%v[[%v]]\n", remote.Config().Name, server.Name().Short(), server.Name().Short()))
			}
			markdown.WriteString("end\n")
			markdown.WriteString(fmt.Sprintf("style %v fill:#1cb8e8,color:#000000\n", remote.Config().Name))
			for _, server := range servers {
				if server.Name() == "HEAD" {
					continue
				}
				commitHash := server.Hash().String()[:4]
				markdown.WriteString(fmt.Sprintf("%v%v[[%v]]-->%v(((%v)))\n", remote.Config().Name, server.Name().Short(), server.Name().Short(), commitHash, commitHash))
			}
		}

	}
	return nil
}

func GenerateTreeEntries(tree *object.Tree, markdown *os.File, fContent *bool) error {
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
				contentText := TruncateString(content, 10)
				markdown.WriteString(fmt.Sprintf("%v--%v-->%v[%v %v]\n", treeHash, file.Name, blobHash, blobHash, contentText))
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
			GenerateTreeEntries(subTree, markdown, fContent)
		}
	}
	return nil
}

func TruncateString(str string, length int) string {
	if length <= 0 {
		return ""
	}

	truncated := ""
	count := 0
	for _, char := range str {
		if char == '\n' {
			break
		}
		truncated += string(char)
		count++
		if count >= length {
			break
		}
	}
	return truncated
}
