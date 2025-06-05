package spv

import (
	"bytes"
	"context"
	"math/big"
	"strings"

	ethereum "github.com/pgprotocol/pgp-chain"
	"github.com/pgprotocol/pgp-chain/accounts/abi"
	"github.com/pgprotocol/pgp-chain/common"
	"github.com/pgprotocol/pgp-chain/ethclient"
	"github.com/pgprotocol/pgp-chain/log"
	"github.com/pgprotocol/pgp-chain/params"
)

// ELAMinterABI is the input ABI used to generate the binding from.
var ELAMinterABIMetaData = "[{\"inputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"constructor\"},{\"inputs\":[],\"name\":\"ReentrancyGuardReentrantCall\",\"type\":\"error\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"internalType\":\"string\",\"name\":\"_addr\",\"type\":\"string\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"_amount\",\"type\":\"uint256\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"_crosschainamount\",\"type\":\"uint256\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"_sender\",\"type\":\"address\"}],\"name\":\"PayloadReceived\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"internalType\":\"bytes32\",\"name\":\"_elaHash\",\"type\":\"bytes32\"},{\"indexed\":true,\"internalType\":\"address\",\"name\":\"_target\",\"type\":\"address\"},{\"indexed\":false,\"internalType\":\"uint256\",\"name\":\"amount\",\"type\":\"uint256\"}],\"name\":\"Recharged\",\"type\":\"event\"},{\"inputs\":[],\"name\":\"ELACoin\",\"outputs\":[{\"internalType\":\"contractIELACoin\",\"name\":\"\",\"type\":\"address\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"elaHash\",\"type\":\"bytes32\"}],\"name\":\"Recharge\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"\",\"type\":\"bytes32\"}],\"name\":\"completed\",\"outputs\":[{\"internalType\":\"bool\",\"name\":\"\",\"type\":\"bool\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes\",\"name\":\"data\",\"type\":\"bytes\"}],\"name\":\"decodeRechargeData\",\"outputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"targetAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"targetAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"fee\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"targetData\",\"type\":\"bytes\"}],\"internalType\":\"structELAMinter.RechargeData[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"pure\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"bytes32\",\"name\":\"elaHash\",\"type\":\"bytes32\"}],\"name\":\"getRechargeData\",\"outputs\":[{\"components\":[{\"internalType\":\"address\",\"name\":\"targetAddress\",\"type\":\"address\"},{\"internalType\":\"uint256\",\"name\":\"targetAmount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"fee\",\"type\":\"uint256\"},{\"internalType\":\"bytes\",\"name\":\"targetData\",\"type\":\"bytes\"}],\"internalType\":\"structELAMinter.RechargeData[]\",\"name\":\"\",\"type\":\"tuple[]\"}],\"stateMutability\":\"view\",\"type\":\"function\"},{\"inputs\":[{\"internalType\":\"string\",\"name\":\"_addr\",\"type\":\"string\"},{\"internalType\":\"uint256\",\"name\":\"_amount\",\"type\":\"uint256\"},{\"internalType\":\"uint256\",\"name\":\"_fee\",\"type\":\"uint256\"}],\"name\":\"withdraw\",\"outputs\":[],\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"stateMutability\":\"payable\",\"type\":\"receive\"}]"
var ELAMinterABI abi.ABI
var ELAMinterAddress = params.ELAMINTER

func init() {
	minterABI, err := abi.JSON(strings.NewReader(ELAMinterABIMetaData))
	if err != nil {
		panic(err)
	}
	ELAMinterABI = minterABI
}

func IsWithdrawTx(input []byte, to *common.Address) bool {
	if to.String() != ELAMinterAddress.String() {
		return false
	}
	method, exist := ELAMinterABI.Methods["withdraw"]
	if !exist {
		return false
	}
	return bytes.HasPrefix(input, method.ID)
}

func IsRechargeTx(input []byte, to common.Address) bool {
	if to != ELAMinterAddress {
		return false
	}
	method, exist := ELAMinterABI.Methods["Recharge"]
	if !exist {
		return false
	}
	return bytes.HasPrefix(input, method.ID)
}

func GetRechargeData(elaHash string) []byte {
	hash := common.HexToHash(elaHash)
	inputData, err := ELAMinterABI.Pack("Recharge", hash)
	if err != nil {
		panic(err)
	}
	return inputData
}

func IsCompleted(elaHash string, ipclient *ethclient.Client) bool {
	hash := common.HexToHash(elaHash)
	input, err := ELAMinterABI.Pack("completed", hash)
	if err != nil {
		panic(err)
	}

	msg := ethereum.CallMsg{From: common.HexToAddress("0x00"), To: &ELAMinterAddress, Data: input}
	out, err := ipclient.CallContract(context.Background(), msg, nil)
	if err != nil {
		log.Error("IsCompleted", "error", err, "out", out)
		return false
	}
	if (out == nil) || (len(out) == 0) {
		return false
	}
	res := big.NewInt(0).SetBytes(out)
	return res.Cmp(big.NewInt(1)) == 0
}
