// Copyright 2014 The pgp-chain Authors
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

package core

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/pgprotocol/pgp-chain/chainbridge_abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/core/types"
	"github.com/pgprotocol/pgp-chain/core/vm"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/params"
	"github.com/pgprotocol/pgp-chain/spv"

	elatx "github.com/elastos/Elastos.ELA/core/transaction"
)

var (
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

/*
The State Transitioning Model

A state transition is a change made when a transaction is applied to the current world state
The state transitioning model does all the necessary work to work out a valid new state root.

1) Nonce handling
2) Pre pay gas
3) Create a new state object if the recipient is \0*32
4) Value transfer
== If contract creation ==

	4a) Attempt to run transaction data
	4b) If valid, use result as code for the new state object

== end ==
5) Run Script section
6) Derive new state root
*/
type StateTransition struct {
	gp         *GasPool
	msg        Message
	gas        uint64
	gasPrice   *big.Int
	initialGas uint64
	value      *big.Int
	data       []byte
	state      vm.StateDB
	evm        *vm.EVM
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() uint64
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
	AccessList() types.AccessList
}

// ExecutionResult includes all output after executing given evm
// message no matter the execution itself is successful or not.
type ExecutionResult struct {
	UsedGas    uint64 // Total used gas but include the refunded gas
	Err        error  // Any error encountered during the execution(listed in core/vm/errors.go)
	ReturnData []byte // Returned data from evm(function result or data supplied with revert opcode)
}

// Unwrap returns the internal evm error which allows us for further
// analysis outside.
func (result *ExecutionResult) Unwrap() error {
	return result.Err
}

// Failed returns the indicator whether the execution is successful or not
func (result *ExecutionResult) Failed() bool { return result.Err != nil }

// Return is a helper function to help caller distinguish between revert reason
// and function return. Return returns the data after execution if no error occurs.
func (result *ExecutionResult) Return() []byte {
	if result.Err != nil {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

// Revert returns the concrete revert reason if the execution is aborted by `REVERT`
// opcode. Note the reason can be nil if no data supplied with revert opcode.
func (result *ExecutionResult) Revert() []byte {
	if result.Err != vm.ErrExecutionReverted {
		return nil
	}
	return common.CopyBytes(result.ReturnData)
}

// IntrinsicGas computes the 'intrinsic gas' for a message with the given data.
func IntrinsicGas(data []byte, contractCreation, isEIP155 bool, isEIP2028 bool) (uint64, error) {
	// Set the starting gas for the raw transaction
	var gas uint64
	if contractCreation && isEIP155 {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	//rawTxid, _, _, _ := spv.IsSmallCrossTxByData(data)
	//if rawTxid != "" {
	//	return gas, nil
	//}
	// Bump the required gas by the amount of transactional data
	if len(data) > 0 {
		// Zero and non-zero bytes are priced differently
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		// Make sure we don't exceed uint64 for all data combinations
		nonZeroGas := params.TxDataNonZeroGasFrontier
		if isEIP2028 {
			nonZeroGas = params.TxDataNonZeroGasEIP2028
		}
		if (math.MaxUint64-gas)/nonZeroGas < nz {
			return 0, vm.ErrOutOfGas
		}
		gas += nz * nonZeroGas

		z := uint64(len(data)) - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, vm.ErrOutOfGas
		}
		gas += z * params.TxDataZeroGas
	}
	return gas, nil
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool) *StateTransition {
	return &StateTransition{
		gp:       gp,
		evm:      evm,
		msg:      msg,
		gasPrice: msg.GasPrice(),
		value:    msg.Value(),
		data:     msg.Data(),
		state:    evm.StateDB,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool) (*ExecutionResult, error) {
	return NewStateTransition(evm, msg, gp).TransitionDb()
}

// to returns the recipient of the message.
func (st *StateTransition) to() common.Address {
	if st.msg == nil || st.msg.To() == nil /* contract creation */ {
		return common.Address{}
	}
	return *st.msg.To()
}

func (st *StateTransition) useGas(amount uint64) error {
	if st.gas < amount {
		return fmt.Errorf("%w: have %d, want %d", ErrIntrinsicGas, st.gas, amount)
	}
	st.gas -= amount

	return nil
}

func (st *StateTransition) buyGas() error {
	mgval := new(big.Int).Mul(new(big.Int).SetUint64(st.msg.Gas()), st.gasPrice)
	if st.state.GetBalance(st.msg.From()).Cmp(mgval) < 0 {
		return errInsufficientBalanceForGas
	}
	if err := st.gp.SubGas(st.msg.Gas()); err != nil {
		return err
	}
	st.gas += st.msg.Gas()

	st.initialGas = st.msg.Gas()
	st.state.SubBalance(st.msg.From(), mgval)
	return nil
}

func (st *StateTransition) preCheck() error {
	// Make sure this transaction's nonce is correct.
	if st.msg.CheckNonce() {
		nonce := st.state.GetNonce(st.msg.From())
		if nonce < st.msg.Nonce() {
			return ErrNonceTooHigh
		} else if nonce > st.msg.Nonce() {
			return ErrNonceTooLow
		}
	}
	return st.buyGas()
}

// TransitionDb will transition the state by applying the current message and
// returning the result including the used gas. It returns an error if failed.
// An error indicates a consensus issue.
func (st *StateTransition) TransitionDb() (result *ExecutionResult, err error) {
	var (
		evm = st.evm
		// vm errors do not effect consensus and are therefor
		// not assigned to err, except for insufficient balance
		// error.
		ret      []byte
		vmerr    error
		snapshot = evm.StateDB.Snapshot()
		rules    = st.evm.ChainConfig().Rules(st.evm.Context.BlockNumber, st.evm.Context.Random != nil, st.evm.Context.Time.Uint64())
	)
	result = &ExecutionResult{0, nil, nil}
	msg := st.msg
	sender := vm.AccountRef(msg.From())
	contractCreation := msg.To() == nil
	isRefundWithdrawTx := spv.IsRefundWithdrawTx(msg.Data(), msg.To())
	isRechargeTx := spv.IsRechargeTx(msg.Data(), msg.To())
	elaHash := ""
	//recharge tx and widthdraw refund
	if isRechargeTx || isRefundWithdrawTx {
		completed := false
		completed, elaHash = spv.IsCompletedByTxInput(msg.Data())
		if completed {
			return &ExecutionResult{0, nil, nil}, ErrMainTxHashCompleted
		}
		st.state.AddBalance(st.msg.From(), new(big.Int).SetUint64(evm.ChainConfig().PassBalance))
		defer func() {
			if vmerr != nil || err != nil {
				evm.StateDB.RevertToSnapshot(snapshot)
				return
			}
			nowBalance := st.state.GetBalance(msg.From())
			if nowBalance.Cmp(new(big.Int).SetUint64(evm.ChainConfig().PassBalance)) < 0 {
				ret = nil
				result.UsedGas = 0
				result.Err = nil
				result.ReturnData = ret
				if err == nil {
					err = ErrGasLimitReached
				}
				evm.StateDB.RevertToSnapshot(snapshot)
			} else {
				st.state.SubBalance(st.msg.From(), new(big.Int).SetUint64(evm.ChainConfig().PassBalance))
			}
		}()
	}
	if err = st.preCheck(); err != nil {
		return nil, err
	}
	homestead := st.evm.ChainConfig().IsHomestead(st.evm.BlockNumber)
	istanbul := st.evm.ChainConfig().IsIstanbul(st.evm.BlockNumber)

	// Pay intrinsic gas
	var gas uint64
	gas, err = IntrinsicGas(st.data, contractCreation, homestead, istanbul)
	if err != nil {
		return &ExecutionResult{0, nil, nil}, err
	}
	if err = st.useGas(gas); err != nil {
		return &ExecutionResult{0, nil, nil}, err
	}
	// Check whether the init code size has been exceeded.
	if rules.IsShanghai && contractCreation && len(msg.Data()) > params.MaxInitCodeSize {
		return nil, fmt.Errorf("%w: code size %v limit %v", vm.ErrMaxInitCodeSizeExceeded, len(msg.Data()), params.MaxInitCodeSize)
	}
	// Set up the initial access list.
	if rules.IsBerlin {
		st.state.PrepareAccessList(rules, msg.From(), st.evm.Context.Coinbase, msg.To(), vm.ActivePrecompiles(rules), msg.AccessList())
	}
	if contractCreation {
		ret, _, st.gas, vmerr = evm.Create(sender, st.data, st.gas, st.value)
	} else {
		// Increment the nonce for the next transaction
		st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
		ret, st.gas, vmerr = evm.Call(sender, st.to(), st.data, st.gas, st.value, nil)
	}
	if vmerr != nil {
		if vmerr != vm.ErrCodeStoreOutOfGas {
			log.Info("VM returned with error", "err", vmerr, "ret", string(ret), " elaHash ", elaHash)
		}
		// The only possible consensus-error would be if there wasn't
		// sufficient balance to make the transfer happen. The first
		// balance transfer may never fail.
		if vmerr == vm.ErrInsufficientBalance {
			return &ExecutionResult{0, vmerr, ret}, vmerr
		}
	}

	if (isRechargeTx || isRefundWithdrawTx) && vmerr == nil {
		st.refundCostGas()
	} else {
		st.refundGas()
	}

	minerFee := new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed()), st.gasPrice)

	if isRechargeTx || isRefundWithdrawTx {
		st.state.AddBalance(st.msg.From(), minerFee)
	} else {
		developerAddress := st.evm.ChainConfig().DeveloperContract
		num := len(developerAddress)
		// During the developer split fee period, half of the transaction fee is allocated to the ELA foundation address
		if st.evm.ChainConfig().IsdeveloperSplitfeeTime(st.evm.Time.Uint64()) {
			if num != 1 {
				return &ExecutionResult{st.gasUsed(), vmerr, ret}, vm.ErrDeveloperSplitFee
			}
			//MinerReward 35% OtherReward 65%
			pgFee := big.NewInt(0).Mul(minerFee, big.NewInt(65))
			pgFee = big.NewInt(0).Div(pgFee, big.NewInt(100))
			pgAddress := common.HexToAddress(developerAddress[0])
			st.state.AddBalance(pgAddress, pgFee)
			minerFee = minerFee.Sub(minerFee, pgFee)
		}
		// Allocate the remaining fee to the miner after deducting the cards fee
		st.state.AddBalance(st.evm.Coinbase, minerFee)
	}
	return &ExecutionResult{st.gasUsed(), vmerr, ret}, err
}

func (st *StateTransition) dealSmallCrossTx() (isSmallCrossTx, verifyed bool, txHash string, err error) {
	msg := st.msg
	err = nil
	rawTxid, rawTx, signatures, height := spv.IsSmallCrossTxByData(msg.Data())
	isSmallCrossTx = len(rawTxid) > 0
	if !isSmallCrossTx {
		verifyed = false
		return isSmallCrossTx, verifyed, rawTxid, errors.New("is not small cross transaction")
	}
	verifyed, err = spv.VerifySmallCrossTx(rawTxid, rawTx, signatures, height)
	if err != nil {
		return isSmallCrossTx, verifyed, rawTxid, err
	}
	if !verifyed {
		return isSmallCrossTx, verifyed, rawTxid, err
	}
	buff, err := hex.DecodeString(rawTx)
	if err != nil {
		log.Error("VerifySmallCrossTx DecodeString raw error", "error", err)
		verifyed = false
		return isSmallCrossTx, verifyed, rawTxid, err
	}
	r := bytes.NewReader(buff)
	txn, err := elatx.GetTransactionByBytes(r)
	if err != nil {
		log.Error("[dealSmallCrossTx] Invalid data from GetTransactionByBytes")
		return isSmallCrossTx, verifyed, rawTxid, err
	}
	err = txn.Deserialize(r)
	if err != nil {
		log.Error("[dealSmallCrossTx] Decode transaction error", err.Error())
		verifyed = false
		return isSmallCrossTx, verifyed, rawTxid, err
	}
	spv.NotifySmallCrossTx(txn)
	return isSmallCrossTx, verifyed, rawTxid, err
}

func (st *StateTransition) isSetArbiterListMethod() (bool, error) {
	eabi, err := chainbridge_abi.GetSetArbitersABI()
	if err != nil {
		return false, err
	}
	method, exist := eabi.Methods["setArbiterList"]
	if !exist {
		return false, errors.New("setArbiterList method not in abi json")
	}

	return bytes.HasPrefix(st.msg.Data(), method.ID), nil
}

func (st *StateTransition) isSetManualArbiterMethod() (bool, error) {
	eabi, err := chainbridge_abi.SetManualArbiterABI()
	if err != nil {
		return false, err
	}
	method, exist := eabi.Methods["setManualArbiter"]
	if !exist {
		return false, errors.New("setManualArbiter method not in abi json")
	}

	return bytes.HasPrefix(st.msg.Data(), method.ID), nil
}

func (st *StateTransition) refundGas() {
	// Apply refund counter, capped to half of the used gas.
	refund := st.gasUsed() / 2
	if refund > st.state.GetRefund() {
		refund = st.state.GetRefund()
	}
	st.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	st.state.AddBalance(st.msg.From(), remaining)

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

func (st *StateTransition) refundCostGas() {
	refund := st.gasUsed()
	st.gas += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	st.state.AddBalance(st.msg.From(), remaining)
	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(st.gas)
}

// gasUsed returns the amount of gas used up by the state transition.
func (st *StateTransition) gasUsed() uint64 {
	return st.initialGas - st.gas
}

// gasUsed returns the amount of gas used up by the state transition.
