// Package leiden provides the project's pure-Go Leiden implementation for
// community detection in weighted, undirected graphs.
//
// The Leiden algorithm (Traag, Waltman, van Eck, 2019) refines the Louvain
// method to guarantee well-connected communities. This package targets the
// graph partitioning primitives needed by the application while remaining
// idiomatic Go with zero external dependencies. It supports CPM and modularity
// optimization, coarsening diagnostics, and recursive size-bounded hierarchies.
//
// The core data structures are:
//
//   - [Edge]: an undirected, weighted edge used as input.
//   - [CompactNetwork]: an immutable CSR-style representation of the graph.
//   - [Clustering]: a partition of nodes into clusters.
//
// See UPSTREAM.md for the imported implementation's source, license, audit,
// and project-specific changes.
package leiden
