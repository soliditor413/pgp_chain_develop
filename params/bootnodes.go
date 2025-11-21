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
	"enode://f9f6e7660492fdf839dc64d5cbe92200a9acd9af8c3ac197f8cab65bd4cc5752f77788f0b08fa96186261bb260f9e7e7d2d601e68897125da6964bd6e4fdb59c@52.62.113.83:0?discport=20670",
	"enode://99d229801465c4d3204590cd7a3986959ce23a3c7a5c924c367b8830ca699a511f37092404a92512747e7de4208eb93b2f3b4ac11ec8a29a124507ba21385b8b@35.156.51.127:0?discport=20670",
	"enode://636a7e1d910e2ba5055db84d92c43d1eddf07b540894d564008e4512833c3e5fdbc419853eaf072d06858330496711592b3ecfd742d19ba02a5327f7f3cfd4fa@35.177.89.244:0?discport=20670",
}

// TestnetBootnodes are the enode URLs of the P2P bootstrap nodes running on the
// Ropsten test network.
var TestnetBootnodes = []string{
	"enode://a9290e617fc2e1ba05a0f66099fab5208214c88bee3b9df86f097160a9eadfb6003acfc9675c9f9d396ba8907687c2cbd40e9e877128e03985f3cb8e725e89b0@13.234.24.155:0?discport=20670",
	"enode://b81f71fe17413aeec2ba0580aa2c7f1542a629eeb6818015029d3b73e2bf018afbe16fd14cba2723c63837accd5612dcca99ff04fcbf43ee446f846cab9df0f9@15.206.198.252:0?discport=20670",
	"enode://c365a4247cf5c27c18fdfd41e4bf313ddb2839d609ab3088386a56c28247901415b3888c5d2392e0d417275a426daf8a3f01b20e3a14a7e79dfd7706e0358a05@13.234.249.168:0?discport=20670",
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
