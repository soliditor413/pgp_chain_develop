// Copyright (c) 2017-2019 The Elastos Foundation
// Use of this source code is governed by an MIT
// license that can be found in the LICENSE file.
//

package msg

import (
	"io"

	"github.com/elastos/Elastos.ELA/common"
	"github.com/elastos/Elastos.ELA/p2p"

	"github.com/pgprotocol/pgp-chain/chainbridge-core/dpos_msg"
)

// Ensure ProducersMsg implement p2p.Message interface.
var _ p2p.Message = (*ProducersMsg)(nil)

type ProducersMsg struct {
	SpvHeight    uint64
	ChangeHeight uint64
	Producers    [][]byte
}

func NewProducersMsg(spvHeight uint64, changeHeight uint64, producers [][]byte) *ProducersMsg {
	return &ProducersMsg{
		SpvHeight:    spvHeight,
		ChangeHeight: changeHeight,
		Producers:    producers,
	}
}

func (msg *ProducersMsg) CMD() string {
	return dpos_msg.CmdProducers
}

func (msg *ProducersMsg) MaxLength() uint32 {
	return 8 + 8 + 8 + (36 * 35)
}

func (msg *ProducersMsg) Serialize(w io.Writer) error {
	err := common.WriteUint64(w, msg.SpvHeight)
	if err != nil {
		return err
	}
	err = common.WriteUint64(w, msg.ChangeHeight)
	if err != nil {
		return err
	}
	count := len(msg.Producers)
	err = common.WriteVarUint(w, uint64(count))
	if err != nil {
		return err
	}
	for _, producer := range msg.Producers {
		err = common.WriteVarBytes(w, producer)
		if err != nil {
			return err
		}
	}
	return nil
}

func (msg *ProducersMsg) Deserialize(r io.Reader) error {
	spvHeight, err := common.ReadUint64(r)
	if err != nil {
		return err
	}
	msg.SpvHeight = spvHeight
	changeHeight, err := common.ReadUint64(r)
	if err != nil {
		return err
	}
	msg.ChangeHeight = changeHeight
	count, err := common.ReadVarUint(r, 0)
	if err != nil {
		return err
	}
	msg.Producers = make([][]byte, count)
	for i := 0; uint64(i) < count; i++ {
		producer, err := common.ReadVarBytes(r, 33, "")
		if err != nil {
			return err
		}
		msg.Producers[i] = producer
	}
	return nil
}
