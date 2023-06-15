package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/proto"
	"github.com/ethereum/go-ethereum/rlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var client proto.EnodeManagerInterfaceClient

func main() {
	if len(os.Args) == 2 {
		nodeIp := os.Args[1]
		clientName := "test"
		dial, err := grpc.Dial(nodeIp, grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)), grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			fmt.Fprintf(os.Stderr, "client.Dial err:%v\n", err)
			return
		}
		client = proto.NewEnodeManagerInterfaceClient(dial)
		streamService, err := client.HandStartStream(context.Background(), &proto.StartStreamReq{Name: clientName})
		if err != nil {
			fmt.Fprintf(os.Stderr, "client.HandStartStream err:%v\n", err)
			return
		}
		go receiveLoop(streamService)

		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)

		select {
		case <-sigint:
			fmt.Fprintf(os.Stdout, "shutting down because of sigint\n")
		}
		// cause future ctrl+c's to panic
		close(sigint)

	} else {
		fmt.Fprintf(os.Stderr, "please input node's ip and port 'receiver 127.0.0.1:1668'\n")
	}
}

func receiveLoop(streamService proto.EnodeManagerInterface_HandStartStreamClient) {
	for {
		//接收服务器流式发送的信息
		recv, err := streamService.Recv()
		currentTime := time.Now()
		//结束接收
		if err == io.EOF {
			fmt.Fprintf(os.Stderr, "end of stream\n")
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "lightSpeedLoop client err:%v\n", err)
			return
		}

		if recv.StreamType == 2 {
			var tx types.Transaction
			err = tx.UnmarshalBinary(recv.Msg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "recv tx err:%v\n", err)
			} else {
				nowTime := time.Now().UnixMilli()
				fmt.Fprintf(os.Stdout, "recv tx:%v time:%s diff:%d\n", tx.Hash(), currentTime.String(), nowTime-recv.StartTiem)
			}
		}
		if recv.StreamType == 1 {
			var block types.Block
			err = block.DecodeRLP(rlp.NewStream(bytes.NewReader(recv.Msg), 0))
			if err != nil {
				fmt.Fprintf(os.Stderr, "recv block err:%v\n", err)
			} else {
				nowTime := time.Now().UnixMilli()
				fmt.Fprintf(os.Stdout, "recv block:%v time:%s diff:%d\n", block.Number(), currentTime.String(), nowTime-recv.StartTiem)
			}
		}
	}
}

func BroadcastTxsToCoinBase(sendTxs []*types.Transaction, coinBase, topic string) {
	sendTxReq := proto.SendTxToCoinBaseReq{}
	sendTxReq.Name = "test"
	sendTxReq.CoinBaseName = coinBase
	sendTxReq.Top = 10
	sendTxReq.TxHex = make([][]byte, 0)
	for _, tx := range sendTxs {
		msg, err := tx.MarshalBinary()
		if err != nil {
			return
		}
		sendTxReq.TxHex = append(sendTxReq.TxHex, msg)
	}
	fmt.Fprintf(os.Stdout, "BroadcastTxsToCoinBase topic:%s To:%s\n", topic, coinBase)
	client.HandSendTxToCoinBase(context.Background(), &sendTxReq)
}

func BroadcastTxsTo(sendTxs []*types.Transaction, topic string) {
	sendTxReq := proto.SendTxReq{}
	sendTxReq.Name = "test"
	sendTxReq.TxHex = make([][]byte, 0)
	for _, tx := range sendTxs {
		msg, err := tx.MarshalBinary()
		if err != nil {
			return
		}
		sendTxReq.TxHex = append(sendTxReq.TxHex, msg)
	}
	fmt.Fprintf(os.Stdout, "BroadcastTxs:%s topic:%s\n", topic)
	client.HandSendTx(context.Background(), &sendTxReq)
}
