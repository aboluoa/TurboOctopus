<h1 align="center">TurboOctopus</h1>

**#Description**

TurboOctopus is a tool developed in Go language based on the BSC (Binance Smart Chain) node client. It enables faster reception of transaction and block information broadcasted by other nodes on the BSC network. Additionally, it facilitates faster broadcasting of your transactions to the miner node with the highest speed closest to you. This tool is particularly useful for sandwich trading bots and arbitrage bots.

**#The disadvantage of bsc node**
1.   The BSC node client does not optimize network connections in order to receive broadcasted information from all connections. Most of the connections are nodes with block heights lower than ours, which are not useful for trade bots；
2. To reduce network transmission, the BSC node client does not broadcast complete messages to all nodes. For block broadcasting, it utilizes NewBlockHashesMsg and NewBlockMsg messages, while for transaction broadcasting, it employs NewPooledTransactionHashesMsg and TransactionsMsg messages. However, the NewBlockHashesMsg and NewPooledTransactionHashesMsg messages do not transmit the complete information. Upon receiving a NewBlockHashesMsg or NewPooledTransactionHashesMsg message, the receiving node needs to check locally if it has received the corresponding hash message before. If not, it sends a request to obtain the complete message content.

   During broadcasting, the node selectively decides whether to send NewBlockMsg or TransactionsMsg messages with complete information. This selection process involves choosing connections from the node's peers pool based on the square root of the total number of connections. You can refer to the "transfer = peers[:int(math.Sqrt(float64(len(peers))))]" code snippet in the eth/handler.go file.

   This design implies that upon receiving a message with a hash, the local node must send a request to retrieve the complete message in order to obtain it. This additional request introduces a delay of several hundred milliseconds or even more in cases of large data volume, resulting in latency for local data retrieval;

3. When broadcasting local transactions, they are not sent to the fastest node. Instead, the broadcasting follows the method described in the second point above. If your node receives a hash message that it has originated, it needs to request the complete message again, resulting in increased transmission latency.

**#Key features**
1. Optimize P2P connection management by automatically kicking out some unuseful connections.
2. Automatically calculate a list of nodes that are potentially closest to a specific miner, enabling faster packaging of transactions by miners when sent locally. This feature typically advances the speed of transaction packaging, which would usually take around 1000 milliseconds before being included in the next miner's block, to sometimes just a few tens or over 100 milliseconds.
3. By continuously crawling P2P node information, the goal is to connect with as many healthy nodes as possible, enabling faster reception of outgoing transactions and connecting with nodes closer to miners.
4. Optimize the handling process of hash messages. The official client's message reception process is not designed for concurrency. TurboOctopus, on the other hand, employs concurrent processing of hash messages to obtain the complete message as quickly as possible.
5. You can specify sending transaction information to a miner whom you wish to promptly include your transactions in their blocks.

**# Building the source**

1. clone the TurboOctopus to local
     ```javascript
   git clone https://github.com/aboluoa/TurboOctopus
   ```
2. build TurboOctopus
     ```javascript
   cd TurboOctopus
   make turbooctopus
   ```
3. build receiver
     ```javascript
   make receiver
   ```

**#Configuration**

Before running TurboOctopus, you need to configure the parameters. The configuration file is located at /build/bin/setting.json.
1. **BootIP** and **BootPort**:These two parameters are the IP address and port of a normal BSC node. TurboOctopus needs to connect to a normal BSC node as the initial crawling entry point. As long as TurboOctopus can establish a connection with this server at the beginning, the server can be disregarded once TurboOctopus is running smoothly. The BSC node does not need to synchronize to the latest height and can serve as the initial entry point as long as it can synchronize data properly.
2. **CrawlCount**:The number of threads for the crawler. This value does not need to be set very high because, in the later stages of crawling, the data is mostly repetitive. The default setting is sufficient.
3. **PendingEnodeCount**: It is recommended to keep the default parameters. This parameter represents the size of a cache pool for the current crawled node information.
4. **MaxCount**:The maximum number of connections in TurboOctopus can be adjusted based on server configuration. For the BSC network, there is no need to set it excessively high since the total number of BSC nodes in the entire network is not very large. The default setting is 2000, which is sufficient for most scenarios.
5. **MaxDailCount**:The number of concurrent connection initiations in TurboOctopus. It is recommended to keep the default parameter value.
6. **PrivateKey**:  This is the private key used by TurboOctopus and other nodes in the handshake protocol. Please avoid using your own wallet's private key for this setting. Generate a random and unused private key for this purpose.
7. **NetworkID**,**GenesisHash**,**HeadNumber**,**HeadHash**,**Td**:These parameters are required for TurboOctopus and other nodes to complete the handshake protocol. It is recommended to keep the default settings because if you are connecting to a normal BSC node, the other party will not kick you out based on your lower block height.


**# Running**
1. Extracting the bsc-geth-empty.tgz file located in the /build/bin directory:
```javascript
cd /build/bin/
tar -zxvf bsc-geth-empty.tgz
```
2. run:
```javascript
cd /build/bin/
./turbooctopus -datadir ./bsc-geth-empty --config ./config_trubo_octopus.toml --pruneancient --syncmode=full
```
2. testing:
    ```javascript
   cd /build/bin/
   ./receiver 127.0.0.1:6686
   ```
   If you can see the following terminal output, it indicates that the software is running properly.
   ```javascript
   recv block:29122737 time:2023-06-15 20:21:03.467657 +0800 CST m=+0.876201670 diff:3
   recv tx:0x9e4c2f2e9ab63e1aefe382469d65618115c1489eaad59495f2fd0e53c6f7bc2f time:2023-06-15 20:21:04.631508 +0800 CST m=+2.040047691 diff:1
   recv tx:0xe147b48b11f530549c8efd43eea644341b3f70061fbcbc58e3a7e1240b214666 time:2023-06-15 20:21:04.784883 +0800 CST m=+2.193422219 diff:1
   recv tx:0x95015473efbe0548120a41285c63f901725b2aed2ca60b118638a3dfa149d9d9 time:2023-06-15 20:21:05.380409 +0800 CST m=+2.788945284 diff:1
   ```