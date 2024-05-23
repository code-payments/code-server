// Package cluster provides utilities for multi-server clustering. It allows nodes
// to coordinate a shared membership state, which can in turn be used to build
// higher level features, such as Distributed Nonce Pools and RPC routing.
//
// In general, an application will create a Cluster, using CreateMembership() to
// register the current process in the cluster. The data of each Member is
// arbitrary, and can be used by the higher level application to coordinate state.
//
// Multiple clusters can be created in a given server process. However, multiple
// Cluster instances for the same real cluster should be avoided.
package cluster
