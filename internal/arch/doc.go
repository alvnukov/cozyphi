// Package arch holds the architecture checks. The boundaries this codebase is
// meant to keep are written down in its tests and checked against the real
// import graph, so a rule that has quietly stopped being true fails a build
// instead of surviving as a paragraph nobody runs.
//
// The package has no code of its own on purpose: a boundary is a fact about
// every other package, so it belongs to none of them.
package arch
