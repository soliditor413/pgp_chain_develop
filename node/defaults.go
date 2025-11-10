// Copyright 2016 The pgp-chain Authors
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

package node

import (
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/pgprotocol/pgp-chain/p2p"
	"github.com/pgprotocol/pgp-chain/p2p/nat"
	"github.com/pgprotocol/pgp-chain/rpc"
)

const (
	DefaultHTTPHost      = "localhost" // Default host interface for the HTTP RPC server
	DefaultHTTPPort      = 20676       // Default TCP port for the HTTP RPC server
	DefaultWSHost        = "localhost" // Default host interface for the websocket RPC server
	DefaultWSPort        = 20675       // Default TCP port for the websocket RPC server
	DefaultGraphQLHost   = "localhost" // Default host interface for the GraphQL server
	DefaultGraphQLPort   = 8557        // Default TCP port for the GraphQL server
	DefaultListenP2pPort = 20678       // Default TCP port for the P2P server
)

// DefaultConfig contains reasonable default settings.
var DefaultConfig = Config{
	DataDir:             DefaultDataDir(),
	HTTPPort:            DefaultHTTPPort,
	HTTPModules:         []string{"net", "web3"},
	HTTPVirtualHosts:    []string{"localhost"},
	HTTPTimeouts:        rpc.DefaultHTTPTimeouts,
	WSPort:              DefaultWSPort,
	WSModules:           []string{"net", "web3"},
	GraphQLPort:         DefaultGraphQLPort,
	GraphQLVirtualHosts: []string{"localhost"},
	P2P: p2p.Config{
		ListenAddr: ":" + strconv.Itoa(DefaultListenP2pPort),
		MaxPeers:   50,
		NAT:        nat.Any(),
	},
}

var DefaultProducers = []string{
	"03244cbfdbee063261f9285fe028d8841cd5a4c4617fa285fa7a95dfedd20c3e5e",
	"03574acf5b9886eacdbfdeda46deabea107d1bfec11a400b0fdf1d79475fa74e01",
	"03830d4d3718e021289b3b0df1b0465c5cae4b403da403b1346dc42e7f0ae9461e",
	"03364106ea544e1c1175dea1ef487b5b56aa48ae680c303ea52631e31d6e5cd438",
	"022909c7d85c88d4d2a8091e279e5a800d2611a4f112019818fec4880a598b64e0",
	"0213d2ad8f4a167f12dd9056dd56c47b2d688277ce909c2bc64401fce6ae9290c3",
	"028d6bbd5965022e1e7263e65193342e43c2569f3b3c7bfa1b088122a7fb7fd925",
	"03b21a599807f516a3e7c00f1e402ce83e72482120f33d24577be5174117a94b7c",
	"0219accb8de9f2f2f5e12068b43552fa4a8118e223389e63585dfc10b33682133d",
	"033cb3eb2442862d37b729b9cafc310883078930e8554b1a0f95f70d50a0061454",
	"03e2283f3b5124bf55bbf4ea4734b493a3524d4b8d00c7b5107f52fcb235cf8069",
	"03361e8f72aed38135aa5ae96f68a95911de6710e2a0218c820844d52c5ee13304",
}

// DefaultDataDir is the default data directory to use for the databases and other
// persistence requirements.
func DefaultDataDir() string {
	// Try to place the data folder in the user's home dir
	home := homeDir()
	if home != "" {
		switch runtime.GOOS {
		case "darwin":
			return filepath.Join(home, "Library", "PGP_Ethereum")
		case "windows":
			// We used to put everything in %HOME%\AppData\Roaming, but this caused
			// problems with non-typical setups. If this fallback location exists and
			// is non-empty, use it, otherwise DTRT and check %LOCALAPPDATA%.
			fallback := filepath.Join(home, "AppData", "Roaming", "PGP_Ethereum")
			appdata := windowsAppData()
			if appdata == "" || isNonEmptyDir(fallback) {
				return fallback
			}
			return filepath.Join(appdata, "PGP_Ethereum")
		default:
			return filepath.Join(home, ".PGP_Ethereum")
		}
	}
	// As we cannot guess a stable location, return empty and handle later
	return ""
}

func windowsAppData() string {
	v := os.Getenv("LOCALAPPDATA")
	if v == "" {
		// Windows XP and below don't have LocalAppData. Crash here because
		// we don't support Windows XP and undefining the variable will cause
		// other issues.
		panic("environment variable LocalAppData is undefined")
	}
	return v
}

func isNonEmptyDir(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	names, _ := f.Readdir(1)
	f.Close()
	return len(names) > 0
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}
