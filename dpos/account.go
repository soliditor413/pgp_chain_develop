// Copyright (c) 2017-2019 The Elastos Foundation
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.
//

package dpos

import (
	"github.com/elastos/Elastos.ELA/account"
	daccount "github.com/elastos/Elastos.ELA/dpos/account"
	"github.com/pgprotocol/pgp-chain/chainbridge-core/crypto"
	"github.com/pgprotocol/pgp-chain/chainbridge-core/crypto/secp256k1"
)

func GetDposAccount(keystorePath string, password []byte) (daccount.Account, error) {
	client, err := account.Open(keystorePath, password)
	if err != nil {
		return nil, err
	}
	return daccount.New(client.GetMainAccount()), nil
}

func GetBridgeAccount(keystorePath string, password []byte) (crypto.Keypair, error) {
	client, err := account.Open(keystorePath, password)
	if err != nil {
		return nil, err
	}
	return secp256k1.NewKeypairFromPrivateKey(client.GetMainAccount().PrivateKey)
}
