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
var ELAMinterABIMetaData = "[\n    {\n      \"inputs\": [],\n      \"stateMutability\": \"nonpayable\",\n      \"type\": \"constructor\"\n    },\n    {\n      \"inputs\": [],\n      \"name\": \"ReentrancyGuardReentrantCall\",\n      \"type\": \"error\"\n    },\n    {\n      \"anonymous\": false,\n      \"inputs\": [\n        {\n          \"indexed\": false,\n          \"internalType\": \"string\",\n          \"name\": \"_addr\",\n          \"type\": \"string\"\n        },\n        {\n          \"indexed\": false,\n          \"internalType\": \"uint256\",\n          \"name\": \"_amount\",\n          \"type\": \"uint256\"\n        },\n        {\n          \"indexed\": false,\n          \"internalType\": \"uint256\",\n          \"name\": \"_crosschainamount\",\n          \"type\": \"uint256\"\n        },\n        {\n          \"indexed\": true,\n          \"internalType\": \"address\",\n          \"name\": \"_sender\",\n          \"type\": \"address\"\n        }\n      ],\n      \"name\": \"PayloadReceived\",\n      \"type\": \"event\"\n    },\n    {\n      \"anonymous\": false,\n      \"inputs\": [\n        {\n          \"indexed\": true,\n          \"internalType\": \"bytes32\",\n          \"name\": \"_elaHash\",\n          \"type\": \"bytes32\"\n        },\n        {\n          \"indexed\": true,\n          \"internalType\": \"address\",\n          \"name\": \"_target\",\n          \"type\": \"address\"\n        },\n        {\n          \"indexed\": false,\n          \"internalType\": \"uint256\",\n          \"name\": \"amount\",\n          \"type\": \"uint256\"\n        },\n        {\n          \"indexed\": false,\n          \"internalType\": \"bytes\",\n          \"name\": \"smallRechargeData\",\n          \"type\": \"bytes\"\n        }\n      ],\n      \"name\": \"Recharged\",\n      \"type\": \"event\"\n    },\n    {\n      \"anonymous\": false,\n      \"inputs\": [\n        {\n          \"indexed\": true,\n          \"internalType\": \"bytes32\",\n          \"name\": \"_withdrawTxID\",\n          \"type\": \"bytes32\"\n        },\n        {\n          \"indexed\": true,\n          \"internalType\": \"address\",\n          \"name\": \"_target\",\n          \"type\": \"address\"\n        },\n        {\n          \"indexed\": false,\n          \"internalType\": \"uint256\",\n          \"name\": \"amount\",\n          \"type\": \"uint256\"\n        },\n        {\n          \"indexed\": false,\n          \"internalType\": \"bytes\",\n          \"name\": \"signatures\",\n          \"type\": \"bytes\"\n        }\n      ],\n      \"name\": \"RefundWithdraw\",\n      \"type\": \"event\"\n    },\n    {\n      \"inputs\": [\n        {\n          \"internalType\": \"bytes32\",\n          \"name\": \"elaHash\",\n          \"type\": \"bytes32\"\n        },\n        {\n          \"internalType\": \"bytes\",\n          \"name\": \"smallRechargeData\",\n          \"type\": \"bytes\"\n        }\n      ],\n      \"name\": \"Recharge\",\n      \"outputs\": [],\n      \"stateMutability\": \"nonpayable\",\n      \"type\": \"function\"\n    },\n    {\n      \"inputs\": [],\n      \"name\": \"_ELACoin\",\n      \"outputs\": [\n        {\n          \"internalType\": \"contract IELACoin\",\n          \"name\": \"\",\n          \"type\": \"address\"\n        }\n      ],\n      \"stateMutability\": \"view\",\n      \"type\": \"function\"\n    },\n    {\n      \"inputs\": [\n        {\n          \"internalType\": \"bytes32\",\n          \"name\": \"\",\n          \"type\": \"bytes32\"\n        }\n      ],\n      \"name\": \"completed\",\n      \"outputs\": [\n        {\n          \"internalType\": \"bool\",\n          \"name\": \"\",\n          \"type\": \"bool\"\n        }\n      ],\n      \"stateMutability\": \"view\",\n      \"type\": \"function\"\n    },\n    {\n      \"inputs\": [\n        {\n          \"internalType\": \"bytes\",\n          \"name\": \"data\",\n          \"type\": \"bytes\"\n        }\n      ],\n      \"name\": \"decodeRechargeData\",\n      \"outputs\": [\n        {\n          \"components\": [\n            {\n              \"internalType\": \"address\",\n              \"name\": \"targetAddress\",\n              \"type\": \"address\"\n            },\n            {\n              \"internalType\": \"uint256\",\n              \"name\": \"targetAmount\",\n              \"type\": \"uint256\"\n            },\n            {\n              \"internalType\": \"uint256\",\n              \"name\": \"fee\",\n              \"type\": \"uint256\"\n            },\n            {\n              \"internalType\": \"bytes\",\n              \"name\": \"targetData\",\n              \"type\": \"bytes\"\n            }\n          ],\n          \"internalType\": \"struct ELAMinter.RechargeData[]\",\n          \"name\": \"\",\n          \"type\": \"tuple[]\"\n        }\n      ],\n      \"stateMutability\": \"pure\",\n      \"type\": \"function\"\n    },\n    {\n      \"inputs\": [\n        {\n          \"internalType\": \"bytes32\",\n          \"name\": \"elaHash\",\n          \"type\": \"bytes32\"\n        }\n      ],\n      \"name\": \"getRechargeData\",\n      \"outputs\": [\n        {\n          \"components\": [\n            {\n              \"internalType\": \"address\",\n              \"name\": \"targetAddress\",\n              \"type\": \"address\"\n            },\n            {\n              \"internalType\": \"uint256\",\n              \"name\": \"targetAmount\",\n              \"type\": \"uint256\"\n            },\n            {\n              \"internalType\": \"uint256\",\n              \"name\": \"fee\",\n              \"type\": \"uint256\"\n            },\n            {\n              \"internalType\": \"bytes\",\n              \"name\": \"targetData\",\n              \"type\": \"bytes\"\n            }\n          ],\n          \"internalType\": \"struct ELAMinter.RechargeData[]\",\n          \"name\": \"\",\n          \"type\": \"tuple[]\"\n        }\n      ],\n      \"stateMutability\": \"view\",\n      \"type\": \"function\"\n    },\n    {\n      \"inputs\": [\n        {\n          \"internalType\": \"bytes32\",\n          \"name\": \"withdrwTxID\",\n          \"type\": \"bytes32\"\n        }\n      ],\n      \"name\": \"getWithdrawData\",\n      \"outputs\": [\n        {\n          \"internalType\": \"address\",\n          \"name\": \"\",\n          \"type\": \"address\"\n        },\n        {\n          \"internalType\": \"uint256\",\n          \"name\": \"\",\n          \"type\": \"uint256\"\n        },\n        {\n          \"internalType\": \"bytes\",\n          \"name\": \"\",\n          \"type\": \"bytes\"\n        }\n      ],\n      \"stateMutability\": \"view\",\n      \"type\": \"function\"\n    },\n    {\n      \"inputs\": [\n        {\n          \"internalType\": \"bytes32\",\n          \"name\": \"withdrwTxID\",\n          \"type\": \"bytes32\"\n        }\n      ],\n      \"name\": \"refundWithdraw\",\n      \"outputs\": [],\n      \"stateMutability\": \"nonpayable\",\n      \"type\": \"function\"\n    },\n    {\n      \"inputs\": [\n        {\n          \"internalType\": \"string\",\n          \"name\": \"_addr\",\n          \"type\": \"string\"\n        },\n        {\n          \"internalType\": \"uint256\",\n          \"name\": \"_amount\",\n          \"type\": \"uint256\"\n        },\n        {\n          \"internalType\": \"uint256\",\n          \"name\": \"_fee\",\n          \"type\": \"uint256\"\n        }\n      ],\n      \"name\": \"withdraw\",\n      \"outputs\": [],\n      \"stateMutability\": \"nonpayable\",\n      \"type\": \"function\"\n    },\n    {\n      \"stateMutability\": \"payable\",\n      \"type\": \"receive\"\n    }\n  ]"
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
	if nil == to {
		return false
	}
	if to.String() != ELAMinterAddress.String() {
		return false
	}
	method, exist := ELAMinterABI.Methods["withdraw"]
	if !exist {
		return false
	}
	return bytes.HasPrefix(input, method.ID)
}

func IsRechargeTx(input []byte, to *common.Address) bool {
	if to == nil {
		return false
	}
	if to.String() != ELAMinterAddress.String() {
		return false
	}
	method, exist := ELAMinterABI.Methods["Recharge"]
	if !exist {
		return false
	}
	return bytes.HasPrefix(input, method.ID)
}

func IsRefundWithdrawTx(input []byte, to *common.Address) bool {
	if to == nil {
		return false
	}
	if to.String() != ELAMinterAddress.String() {
		return false
	}
	method, exist := ELAMinterABI.Methods["refundWithdraw"]
	if !exist {
		return false
	}
	return bytes.HasPrefix(input, method.ID)
}

func GetRechargeData(elaHash string, smallCrossTx []byte) []byte {
	hash := common.HexToHash(elaHash)
	inputData, err := ELAMinterABI.Pack("Recharge", hash, smallCrossTx)
	if err != nil {
		panic(err)
	}
	return inputData
}

func GetRefundWithdrawData(withdrawHash string) []byte {
	hash := common.HexToHash(withdrawHash)
	inputData, err := ELAMinterABI.Pack("refundWithdraw", hash)
	if err != nil {
		panic(err)
	}
	return inputData
}

func IsCompleted(elaHashOrWithdrawHash string, ipclient *ethclient.Client) bool {
	hash := common.HexToHash(elaHashOrWithdrawHash)
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
