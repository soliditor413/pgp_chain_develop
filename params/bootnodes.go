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
	"enode://8c367269247cf63f41b7047e7fc2567fe84f952332fc6c612610c7b84b9a556fd9354b41ab7d8469ad86560210d1e8f5a09a398b092e78249f820ea17a9ad796@52.62.113.83:0?discport=20660",
	"enode://6b3db576ef73e01979a6f0f9b6c0c94be85ba72778e1fe6edde850816d8a1918fd7d03a726004cbd4120ca5b7747d6079a2838952730054dc61360bb32365bed@35.156.51.127:0?discport=20660",
	"enode://f46954381d97cc03bbb8f177894e46af43bebcb1dbfaf78d5de87280caaa254428ba37c06436c1d0fce38e4b1cee24c49dbe6fe0f39ec9063599dc250c1d0fd8@35.177.89.244:0?discport=20660",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://f7ecb7705471b103d2e6aee61427f014e6f3c658e4e28416b7a96aebfb180c83869e46312e19e69711db15cdedef9e2ed1002bb9d8c5af634c43d26e3a6eca7a@13.234.24.155:20660",
	"enode://138f5bddd685b8bdd203075499f48f022894cd95041e89812dd5160439f196af36869dc5d8cdb97e508ad9c9e4e80511a93707a65badda1a93dc18252f3cffab@15.206.198.252:20660",
	"enode://e1a54ff3f8e3582d0fd7418024bf67b2ede860080b2f3cd450f856d94d8c9d8972eee0885a62d7d62d96201b90e47610e13922f9e410674e5a1b80af868bf422@13.234.249.168:20660",
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
