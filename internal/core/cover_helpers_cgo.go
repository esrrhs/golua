//go:build cgo && unix

package core

func coverWarnfSeesUD(L *State) { _ = testWarnfSeesUD(L) }
