package main

import (
	"flag"
	"fmt"
	"log"
	"os"

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
	flag.Parse()

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

	_, err = commiter.Next()
	if err != nil {
		markdown.WriteString("empty((empty))\n")
		return
	}

	commiter.ForEach(func(c *object.Commit) error {

		commitHash := c.Hash.String()[:4]

		// Tree and Blob
		if *fTree {
			treeHash := c.TreeHash.String()[:4]
			markdown.WriteString(fmt.Sprintf("%v(((%v)))-->%v{%v}\n", commitHash, commitHash, treeHash, treeHash))
			tree, err := c.Tree()
			if err != nil {
				return err
			}
			if *fBlob {
				tree.Files().ForEach(func(f *object.File) error {
					blobHash := f.Hash.String()[:4]
					content, err := f.Contents()
					if err != nil {
						return err
					}
					markdown.WriteString(fmt.Sprintf("%v--%v-->%v[%v %v]\n", treeHash, f.Name, blobHash, blobHash, content))
					return nil
				})
			}
		} else {
			markdown.WriteString(fmt.Sprintf("%v(((%v)))\n", commitHash, commitHash))
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

	// Branch and Head
	if *fBranch {
		branches, err := repo.Branches()
		if err != nil {
			return
		}
		branches.ForEach(func(r *plumbing.Reference) error {
			markdown.WriteString(fmt.Sprintf("%v[[%v]]-->%v\n", r.Name().Short(), r.Name().Short(), r.Hash().String()[:4]))
			return nil
		})
	}

	if *fHead {
		head, err := repo.Head()
		if err != nil {
			return
		}
		markdown.WriteString(fmt.Sprintf("HEAD{{HEAD}}-->%v\n", head.Hash().String()[:4]))
	}
}
