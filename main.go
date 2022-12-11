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
				return listTreeEntries(tree, markdown)
			}
		} else {
			if *fBlob {
				tree.Files().ForEach(func(f *object.File) error {
					blobHash := f.Hash.String()[:4]
					content, err := f.Contents()
					if err != nil {
						return err
					}
					markdown.WriteString(fmt.Sprintf("%v(((%v)))--%v-->%v[%v %v]\n", commitHash, commitHash, f.Name, blobHash, blobHash, content))
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

	head, err := repo.Head()
	if err != nil {
		markdown.WriteString("empty((empty))\n")
		return
	}

	if *fHead {
		if head.Name().IsBranch() && *fBranch {
			markdown.WriteString(fmt.Sprintf("HEAD{{HEAD}}-->%v\n", head.Name().Short()))
		} else {
			markdown.WriteString(fmt.Sprintf("HEAD{{%v}}-->%v\n", head.Name().Short(), head.Hash().String()[:4]))
		}
	}
}

func listTreeEntries(tree *object.Tree, markdown *os.File) error {
	treeHash := tree.Hash.String()[:4]
	for _, entry := range tree.Entries {
		if entry.Mode.IsFile() {
			file, err := tree.TreeEntryFile(&entry)
			if err != nil {
				return err
			}
			blobHash := file.Hash.String()[:4]
			content, err := file.Contents()
			if err != nil {
				return err
			}
			markdown.WriteString(fmt.Sprintf("%v--%v-->%v[%v %v]\n", treeHash, file.Name, blobHash, blobHash, content))
		} else {
			subTree, err := tree.Tree(entry.Name)
			if err != nil {
				return err
			}
			treeHash := tree.Hash.String()[:4]
			subTreeHash := subTree.Hash.String()[:4]
			markdown.WriteString(fmt.Sprintf("%v{%v}--%v-->%v{%v}\n", treeHash, treeHash, entry.Name, subTreeHash, subTreeHash))
			listTreeEntries(subTree, markdown)
		}
	}
	return nil
}
