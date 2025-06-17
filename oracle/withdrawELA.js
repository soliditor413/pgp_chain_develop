Web3 = require("web3");
// web3 = new Web3("https://api-testnet.elastos.io/eth");
web3 = new Web3("https://api2-testnet.elastos.io/pgp");

contract = new web3.eth.Contract(
    [
        {
            "inputs": [],
            "stateMutability": "nonpayable",
            "type": "constructor"
        },
        {
            "inputs": [],
            "name": "ReentrancyGuardReentrantCall",
            "type": "error"
        },
        {
            "anonymous": false,
            "inputs": [
                {
                    "indexed": false,
                    "internalType": "string",
                    "name": "_addr",
                    "type": "string"
                },
                {
                    "indexed": false,
                    "internalType": "uint256",
                    "name": "_amount",
                    "type": "uint256"
                },
                {
                    "indexed": false,
                    "internalType": "uint256",
                    "name": "_crosschainamount",
                    "type": "uint256"
                },
                {
                    "indexed": true,
                    "internalType": "address",
                    "name": "_sender",
                    "type": "address"
                }
            ],
            "name": "PayloadReceived",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {
                    "indexed": true,
                    "internalType": "bytes32",
                    "name": "_elaHash",
                    "type": "bytes32"
                },
                {
                    "indexed": true,
                    "internalType": "address",
                    "name": "_target",
                    "type": "address"
                },
                {
                    "indexed": false,
                    "internalType": "uint256",
                    "name": "amount",
                    "type": "uint256"
                },
                {
                    "indexed": false,
                    "internalType": "bytes",
                    "name": "smallRechargeData",
                    "type": "bytes"
                }
            ],
            "name": "Recharged",
            "type": "event"
        },
        {
            "anonymous": false,
            "inputs": [
                {
                    "indexed": true,
                    "internalType": "bytes32",
                    "name": "_withdrawTxID",
                    "type": "bytes32"
                },
                {
                    "indexed": true,
                    "internalType": "address",
                    "name": "_target",
                    "type": "address"
                },
                {
                    "indexed": false,
                    "internalType": "uint256",
                    "name": "amount",
                    "type": "uint256"
                },
                {
                    "indexed": false,
                    "internalType": "bytes",
                    "name": "signatures",
                    "type": "bytes"
                }
            ],
            "name": "RefundWithdraw",
            "type": "event"
        },
        {
            "inputs": [
                {
                    "internalType": "bytes32",
                    "name": "elaHash",
                    "type": "bytes32"
                },
                {
                    "internalType": "bytes",
                    "name": "smallRechargeData",
                    "type": "bytes"
                }
            ],
            "name": "Recharge",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [],
            "name": "_ELACoin",
            "outputs": [
                {
                    "internalType": "contract IELACoin",
                    "name": "",
                    "type": "address"
                }
            ],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [
                {
                    "internalType": "bytes32",
                    "name": "",
                    "type": "bytes32"
                }
            ],
            "name": "completed",
            "outputs": [
                {
                    "internalType": "bool",
                    "name": "",
                    "type": "bool"
                }
            ],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [
                {
                    "internalType": "bytes",
                    "name": "data",
                    "type": "bytes"
                }
            ],
            "name": "decodeRechargeData",
            "outputs": [
                {
                    "components": [
                        {
                            "internalType": "address",
                            "name": "targetAddress",
                            "type": "address"
                        },
                        {
                            "internalType": "uint256",
                            "name": "targetAmount",
                            "type": "uint256"
                        },
                        {
                            "internalType": "uint256",
                            "name": "fee",
                            "type": "uint256"
                        },
                        {
                            "internalType": "bytes",
                            "name": "targetData",
                            "type": "bytes"
                        }
                    ],
                    "internalType": "struct ELAMinter.RechargeData[]",
                    "name": "",
                    "type": "tuple[]"
                }
            ],
            "stateMutability": "pure",
            "type": "function"
        },
        {
            "inputs": [
                {
                    "internalType": "bytes32",
                    "name": "elaHash",
                    "type": "bytes32"
                }
            ],
            "name": "getRechargeData",
            "outputs": [
                {
                    "components": [
                        {
                            "internalType": "address",
                            "name": "targetAddress",
                            "type": "address"
                        },
                        {
                            "internalType": "uint256",
                            "name": "targetAmount",
                            "type": "uint256"
                        },
                        {
                            "internalType": "uint256",
                            "name": "fee",
                            "type": "uint256"
                        },
                        {
                            "internalType": "bytes",
                            "name": "targetData",
                            "type": "bytes"
                        }
                    ],
                    "internalType": "struct ELAMinter.RechargeData[]",
                    "name": "",
                    "type": "tuple[]"
                }
            ],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [
                {
                    "internalType": "bytes32",
                    "name": "withdrwTxID",
                    "type": "bytes32"
                }
            ],
            "name": "getWithdrawData",
            "outputs": [
                {
                    "internalType": "address",
                    "name": "",
                    "type": "address"
                },
                {
                    "internalType": "uint256",
                    "name": "",
                    "type": "uint256"
                },
                {
                    "internalType": "bytes",
                    "name": "",
                    "type": "bytes"
                }
            ],
            "stateMutability": "view",
            "type": "function"
        },
        {
            "inputs": [
                {
                    "internalType": "bytes32",
                    "name": "withdrwTxID",
                    "type": "bytes32"
                }
            ],
            "name": "refundWithdraw",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "inputs": [
                {
                    "internalType": "string",
                    "name": "_addr",
                    "type": "string"
                },
                {
                    "internalType": "uint256",
                    "name": "_amount",
                    "type": "uint256"
                },
                {
                    "internalType": "uint256",
                    "name": "_fee",
                    "type": "uint256"
                }
            ],
            "name": "withdraw",
            "outputs": [],
            "stateMutability": "nonpayable",
            "type": "function"
        },
        {
            "stateMutability": "payable",
            "type": "receive"
        }
    ]
);
contract.options.address = "0x0000000000000000000000000000000000000064";

//提现账号
acc = web3.eth.accounts.decrypt(keystore, "password");

const withDrawValue = 1e8;
const withdawFee = 0.0001 * 1e8;

cdata  = contract.methods.withdraw("EPyWeXwnxqA6MnqmkZ28wSo5rJuq77FsaM", withDrawValue, withdawFee).encodeABI(); //参数 1. 主链地址 2，提现金额 3，手续费

params = "####123test"

const buf = Buffer.from(params, 'utf-8');
paramsHex = buf.toString('hex')
console.log(paramsHex);
cdata = cdata + paramsHex;

tx = {data: cdata, to: contract.options.address, from: acc.address, gas: "38204", gasPrice: 500 * 1e9}
tx.value = withDrawValue;

web3.eth.estimateGas(tx).then((gasLimit)=>{
    tx.gas=gasLimit;
    console.log("gasLimit", gasLimit);
});

acc.signTransaction(tx).then((res)=>{
    console.log("coming");
    stx = res;
    console.log(stx.rawTransaction);
    web3.eth.sendSignedTransaction(stx.rawTransaction).then(console.log)
});