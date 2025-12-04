# Producer 参与统计功能说明

## 功能概述

本功能用于统计和跟踪 PBFT 共识中每个 Producer 的参与情况，包括：
- 最后一次参与共识的时间
- 未参与共识的时长（不活跃时长）
- 参与共识的次数
- 最后一次出块的区块高度

## 实现说明

### 核心模块

1. **`producer_stats.go`**: Producer 统计核心模块
   - `ProducerStats`: 统计数据结构
   - `RecordParticipation()`: 记录 Producer 参与情况
   - `GetInactiveDuration()`: 获取不活跃时长
   - `GetParticipationInfo()`: 获取详细参与信息
   - `GetAllProducersStats()`: 获取所有 Producer 的统计信息

2. **集成点**:
   - 在 `pbft.go` 的 `Pbft` 结构体中添加了 `producerStats` 字段
   - 在 `network.go` 的 `OnBlockReceived()` 方法中，当区块成功插入后自动记录 Producer 参与情况

### API 接口

通过 RPC API 可以查询 Producer 参与统计信息：

#### 1. 获取指定 Producer 的参与信息

```json
{
  "jsonrpc": "2.0",
  "method": "pbft_getProducerParticipationInfo",
  "params": ["<producer_public_key_hex>"],
  "id": 1
}
```

**响应示例**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "producerPublicKey": "02a1b2c3d4e5f6...",
    "lastParticipationTime": "2024-01-15T10:30:00Z",
    "inactiveDuration": 3600000000000,
    "participationCount": 150,
    "lastBlockHeight": 12345,
    "neverParticipated": false
  }
}
```

#### 2. 获取指定 Producer 的不活跃时长（秒）

```json
{
  "jsonrpc": "2.0",
  "method": "pbft_getProducerInactiveDuration",
  "params": ["<producer_public_key_hex>"],
  "id": 1
}
```

**响应示例**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": [3600, false]
}
```

说明：
- 第一个值：不活跃时长（秒）
- 第二个值：`true` 表示从未参与过共识，`false` 表示曾经参与过

#### 3. 获取所有 Producer 的统计信息

```json
{
  "jsonrpc": "2.0",
  "method": "pbft_getAllProducersParticipationStats",
  "params": [],
  "id": 1
}
```

**响应示例**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "02a1b2c3d4e5f6...": {
      "producerPublicKey": "02a1b2c3d4e5f6...",
      "lastParticipationTime": "2024-01-15T10:30:00Z",
      "inactiveDuration": 3600000000000,
      "participationCount": 150,
      "lastBlockHeight": 12345,
      "neverParticipated": false
    },
    "03b2c3d4e5f6a7...": {
      "producerPublicKey": "03b2c3d4e5f6a7...",
      "lastParticipationTime": "2024-01-15T09:00:00Z",
      "inactiveDuration": 7200000000000,
      "participationCount": 200,
      "lastBlockHeight": 12300,
      "neverParticipated": false
    }
  }
}
```

## 使用示例

### 使用 curl 查询

```bash
# 查询指定 Producer 的参与信息
curl -X POST http://localhost:20666 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "pbft_getProducerParticipationInfo",
    "params": ["02a1b2c3d4e5f678901234567890123456789012345678901234567890123456"],
    "id": 1
  }'

# 查询指定 Producer 的不活跃时长
curl -X POST http://localhost:20666 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "pbft_getProducerInactiveDuration",
    "params": ["02a1b2c3d4e5f678901234567890123456789012345678901234567890123456"],
    "id": 1
  }'

# 查询所有 Producer 的统计信息
curl -X POST http://localhost:20666 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "pbft_getAllProducersParticipationStats",
    "params": [],
    "id": 1
  }'
```

### 使用 JavaScript/Node.js

```javascript
const Web3 = require('web3');
const web3 = new Web3('http://localhost:20666');

// 查询指定 Producer 的参与信息
async function getProducerInfo(producerPubKey) {
  const result = await web3.currentProvider.send({
    jsonrpc: '2.0',
    method: 'pbft_getProducerParticipationInfo',
    params: [producerPubKey],
    id: 1
  });
  return result.result;
}

// 查询不活跃时长
async function getInactiveDuration(producerPubKey) {
  const result = await web3.currentProvider.send({
    jsonrpc: '2.0',
    method: 'pbft_getProducerInactiveDuration',
    params: [producerPubKey],
    id: 1
  });
  return result.result;
}

// 查询所有 Producer 统计
async function getAllStats() {
  const result = await web3.currentProvider.send({
    jsonrpc: '2.0',
    method: 'pbft_getAllProducersParticipationStats',
    params: [],
    id: 1
  });
  return result.result;
}
```

## 技术细节

### 数据提取

Producer 的公钥从区块的 `Extra` 字段中提取：
1. 从区块的 `Extra` 字段反序列化 `Confirm` 结构
2. 从 `Confirm.Proposal.Sponsor` 获取 Producer 的公钥

### 记录时机

Producer 参与情况在以下时机记录：
- 当区块成功插入到区块链后（`OnBlockReceived` 方法中）
- 仅记录新产生的区块，不记录历史同步的区块

### 线程安全

所有统计操作都使用 `sync.RWMutex` 保护，确保并发安全。

## 注意事项

1. **统计范围**: 仅统计节点运行期间新产生的区块，不包含历史区块
2. **内存使用**: 统计信息存储在内存中，节点重启后会丢失
3. **Producer 公钥格式**: 使用十六进制编码的字符串格式
4. **时间精度**: 不活跃时长以秒为单位返回

## 未来改进

可以考虑的改进方向：
1. 持久化统计信息到数据库
2. 支持查询历史统计信息
3. 添加统计信息的定期清理机制
4. 支持按时间范围查询统计信息

