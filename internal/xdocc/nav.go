package xdocc

import "strings"

// Nav is one entry of a navigation tree, as templates see it.
type Nav struct {
	Name     string // display name
	URL      string // path from the site root, e.g. "docs/api/index.html"
	Path     string // the directory it points at, e.g. "docs/api"
	Href     string // relative to the page being rendered
	Active   bool   // the current page is this directory or below it
	Current  bool   // the current page is in this directory
	Children []*Nav

	item *Item
}

// navTree builds the navigation below dir, from the point of view of current.
// Only directories marked "nav" are included; a directory that is not in the
// navigation also hides its children from it.
func navTree(dir, current *Item) []*Nav {
	var nodes []*Nav
	for _, child := range dir.Children {
		if !child.IsDir || !child.IsNav() {
			continue
		}
		node := &Nav{
			Name:    child.Name,
			URL:     child.URL,
			Path:    strings.TrimSuffix(child.Dir, "/"),
			Href:    current.Root() + child.URL,
			Current: child == current,
			item:    child,
		}
		node.Active = node.Current || isAncestor(child, current)
		node.Children = navTree(child, current)
		nodes = append(nodes, node)
	}
	return nodes
}

// isAncestor reports whether dir is somewhere above item.
func isAncestor(dir, item *Item) bool {
	for p := item; p != nil; p = p.Parent {
		if p == dir {
			return true
		}
	}
	return false
}

// findNav looks for the navigation entry of dir in a tree.
func findNav(nodes []*Nav, dir *Item) *Nav {
	for _, node := range nodes {
		if node.item == dir {
			return node
		}
		if found := findNav(node.Children, dir); found != nil {
			return found
		}
	}
	return nil
}

// breadcrumb is the chain of directories from the site root down to dir, the
// root itself excluded.
func breadcrumb(dir, current *Item) []*Nav {
	if dir.Parent == nil {
		return nil
	}
	node := &Nav{
		Name:    dir.Name,
		URL:     dir.URL,
		Path:    strings.TrimSuffix(dir.Dir, "/"),
		Href:    current.Root() + dir.URL,
		Current: dir == current,
		Active:  true,
		item:    dir,
	}
	return append(breadcrumb(dir.Parent, current), node)
}
