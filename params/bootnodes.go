// Copyright 2015 The pgp-chain Authors
// This file is part of the pgp-chain library.
//
// The pgp-chain library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The pgp-chain library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the pgp-chain library. If not, see <http://www.gnu.org/licenses/>.

package params

// MainnetBootnodes are the enode URLs of the P2P bootstrap nodes running on
// the main Ethereum network.
var MainnetBootnodes = []string{
	"enode://9a7d82ce819695c63e750618d3822b09f634eece966ca98a7f28a89d711b3bfaa5ecda7c74500e22bc3ea0c63cffac6fbbb13f5d17bd456bb017ebc5446602af@52.62.113.83:0?discport=20670",
	"enode://79a309ebf8a84c5a30c3c18025c84b7400ca21097398fb9bfa20b0c4afa3f1a2911cc9aa2ae5eda839774c70fbb5881b3c6e9292b8bf62a8cbcf795039110928@35.156.51.127:0?discport=20670",
	"enode://ff181cd1afcf6f63c9447f94781ecef0811111725ad3d230a1ade2f8fda24f6e754f1fc4b0d64bdd9f05758be4abe168513b72fbeb84049384b5f3f978906343@35.177.89.244:0?discport=20670",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://f7ecb7705471b103d2e6aee61427f014e6f3c658e4e28416b7a96aebfb180c83869e46312e19e69711db15cdedef9e2ed1002bb9d8c5af634c43d26e3a6eca7a@13.234.24.155:20670",
	"enode://138f5bddd685b8bdd203075499f48f022894cd95041e89812dd5160439f196af36869dc5d8cdb97e508ad9c9e4e80511a93707a65badda1a93dc18252f3cffab@15.206.198.252:20670",
	"enode://e1a54ff3f8e3582d0fd7418024bf67b2ede860080b2f3cd450f856d94d8c9d8972eee0885a62d7d62d96201b90e47610e13922f9e410674e5a1b80af868bf422@13.234.249.168:20670",
}

// RinkebyBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Rinkeby test network.
var RinkebyBootnodes = []string{}

// GoerliBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Görli test network.
var GoerliBootnodes = []string{}

// DiscoveryV5Bootnodes are the enode URLs of the P2P bootstrap nodes for the
// experimental RLPx v5 topic-discovery network.
var DiscoveryV5Bootnodes = []string{}
