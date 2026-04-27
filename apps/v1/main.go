package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os/signal"
	"pkg/model"
	"pkg/util"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/dynamodb/dynamodbattribute"
)

const TableName = "test_table_benchmarks_v1_" + runtime.GOARCH
const CloudWatchNamespace = "Benchmark/GoSDKv1"
const BatchGetLimit = 25
const BatchWriteLimit = 25
const TransactGetLimit = 100
const TransactWriteLimit = 100

func init() {
	// enable CSM if not already enabled
	if util.Env("AWS_CSM_ENABLED", "") != "true" {
		_ = util.SetEnv("AWS_CSM_ENABLED", "true")
	}
	if util.Env("AWS_CSM_HOST", "") == "" {
		_ = util.SetEnv("AWS_CSM_HOST", "127.0.0.1")
	}
	if util.Env("AWS_CSM_PORT", "") == "" {
		_ = util.SetEnv("AWS_CSM_PORT", fmt.Sprintf("%d", 31000+rand.Intn(8999)))
	}
	if util.Env("AWS_CSM_CLIENT_ID", "") == "" {
		_ = util.SetEnv("AWS_CSM_CLIENT_ID", "av-bm-v1")
	}
}

func main() {
	size := util.Size(util.Env("SIZE", "1KB"))
	if !size.Valid() {
		panic(fmt.Sprintf("invalid SIZE: %v, valid values: %v", size, size.Values()))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGHUP)
	defer stop()

	sess := awsSession()
	ddb := dynamoDBService(sess)
	cw := cloudwatchService(sess)

	sender := &util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]{
		Namespace: CloudWatchNamespace,
		Queue:     make(chan model.Event, 1024*1024),
		Sender:    cw.PutMetricData,
		Dimensions: []model.Dimension{
			{Name: util.Pointer(util.DimensionNameSDK), Value: util.Pointer(util.DimensionSDKV1)},
			{Name: util.Pointer(util.DimensionNameSize), Value: util.Pointer(string(size))},
			{Name: util.Pointer(util.DimensionNameOS), Value: util.Pointer(runtime.GOOS)},
			{Name: util.Pointer(util.DimensionNameArch), Value: util.Pointer(runtime.GOARCH)},
		},
	}

	wg := &sync.WaitGroup{}

	// cw worker
	wg.Add(1)
	go sender.Start(ctx, wg)
	// CSM server
	wg.Add(1)
	go mainCSMServer(ctx, wg, sender)
	// ddb worker
	wg.Add(1)
	go mainDynamoDB(ctx, wg, ddb, sender, stop, size)
	wg.Wait()
}

func mainCSMServer(ctx context.Context, wg *sync.WaitGroup, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	defer func() {
		wg.Done()
		log.Println("mainCSMServer exiting")
	}()

	enabled := util.Env("AWS_CSM_ENABLED", "true")
	if enabled != "true" {
		return
	}

	host := util.Env("AWS_CSM_HOST", "127.0.0.1")
	clientId := util.Env("AWS_CSM_CLIENT_ID", "test-client")
	_ = clientId
	port, err := util.EnvInt("AWS_CSM_PORT", 31000)
	if err != nil {
		panic(fmt.Sprintf("invalid AWS_CSM_PORT: %v", err))
	}

	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		panic(fmt.Sprintf("resolve udp addr: %v", err))
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(fmt.Sprintf("listen udp: %v", err))
	}
	defer conn.Close()

	log.Printf("listening for AWS SDK v1 CSM on udp://%s", addr.String())

	buf := make([]byte, 65535)
	for {
		select {
		case <-ctx.Done():
			log.Printf("shutting down: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}
		log.Print("waiting for CSM packet...")
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			println(fmt.Sprintf("read error: %v", err))
			continue
		}

		record := string(buf[:n])
		//log.Printf("received CSM packet from %s (%d bytes): %s", remote, n, record)

		var event model.Event
		if err = json.Unmarshal([]byte(record), &event); err != nil {
			log.Printf("invalid CSM packet from %s (%d bytes): %v", remote, n, err)
			continue
		}

		sender.Send(event)
	}
}

func mainDynamoDB(ctx context.Context, wg *sync.WaitGroup, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput], stop context.CancelFunc, size util.Size) {
	defer func() {
		wg.Done()
		stop()
		log.Println("mainDynamoDB exiting")
	}()

	ensureTable(ddb)

	limit, err := util.EnvInt("LIMIT", 1024)
	if err != nil {
		panic(fmt.Sprintf("invalid LIMIT: %v", err))
	}
	iterations, err := util.EnvInt("ITERATIONS", 1)
	if err != nil {
		panic(fmt.Sprintf("invalid ITERATIONS: %v", err))
	}
	paralellism, err := util.EnvInt("PARALLELISM", 1)
	if err != nil {
		panic(fmt.Sprintf("invalid PARALLELISM: %v", err))
	}

	log.Printf("Running %d iterations with entity limit %d", iterations, limit)

	ddbWg := &sync.WaitGroup{}
	for i := 0; i < paralellism; i++ {
		ddbWg.Add(1)
		go func() {
			defer ddbWg.Done()

			for i := 0; i < iterations; i++ {
				log.Printf("Iteration %d/%d", i+1, iterations)

				// crud (put instead of create)
				runPut(ctx, size, limit, ddb, sender)
				runRead(ctx, size, limit, ddb, sender)
				runQuery(ctx, size, limit, ddb, sender)
				runScan(ctx, size, limit, ddb, sender)
				runUpdate(ctx, size, limit, ddb, sender)
				runDelete(ctx, size, limit, ddb, sender)

				// batches
				runBatchWrite(ctx, size, limit, ddb, sender)
				runBatchRead(ctx, size, limit, ddb, sender)
				runBatchDelete(ctx, size, limit, ddb, sender)

				// transactions
				runTransactPut(ctx, size, limit, ddb, sender)
				runTransactGet(ctx, size, limit, ddb, sender)
				runTransactUpdate(ctx, size, limit, ddb, sender)
				runTransactDelete(ctx, size, limit, ddb, sender)
			}
		}()
	}

	ddbWg.Wait()
}

func runPut(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runPut: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		user := util.GenerateUser(size)

		userMap, err := util.MarshalWithMetrics(ctx, &user, sender, dynamodbattribute.MarshalMap)
		if err != nil {
			log.Printf("runPut(): marshal user %d: %v", l, err)
			continue
		}

		userMap["id"] = &dynamodb.AttributeValue{
			S: aws.String(fmt.Sprintf("ID:%d", l)),
		}
		_, err = ddb.PutItemWithContext(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(TableName),
			Item:      userMap,
		})

		if err != nil {
			log.Printf("runPut(): put item %d: %v", l, err)
		}
	}
}

func runRead(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runRead: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		res, err := ddb.GetItemWithContext(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(TableName),
			Key: map[string]*dynamodb.AttributeValue{
				"id": {
					S: aws.String(fmt.Sprintf("ID:%d", l)),
				},
			},
		})
		if err != nil {
			log.Printf("runRead(): get item %d: %v", l, err)
			continue
		}

		user, err := util.UnmarshalWithMetrics[model.User](ctx, res.Item, sender, dynamodbattribute.UnmarshalMap)
		if err != nil {
			log.Printf("runRead(): unmarshal item %d: %v", l, err)
			continue
		}

		log.Printf("read user %s", user.ID)
	}
}

func runQuery(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		var exclusiveStartKey map[string]*dynamodb.AttributeValue

		for {
			if shouldExit(ctx, "runQuery") {
				return
			}

			res, err := ddb.QueryWithContext(ctx, &dynamodb.QueryInput{
				TableName:              aws.String(TableName),
				KeyConditionExpression: aws.String("id = :id"),
				ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
					":id": {
						S: aws.String(fmt.Sprintf("ID:%d", l)),
					},
				},
				ExclusiveStartKey: exclusiveStartKey,
			})

			if err != nil {
				log.Printf("runQuery(): query item %d: %v", l, err)
				continue
			}

			for _, item := range res.Items {
				user, err := util.UnmarshalWithMetrics[model.User](ctx, item, sender, dynamodbattribute.UnmarshalMap)
				if err != nil {
					log.Printf("runQuery(): unmarshal item %d: %v", l, err)
					continue
				}

				log.Printf("queried user %s", user.ID)
			}

			exclusiveStartKey = res.LastEvaluatedKey
			if len(exclusiveStartKey) == 0 {
				break
			}
		}
	}
}

func runScan(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	var exclusiveStartKey map[string]*dynamodb.AttributeValue

	for {
		if shouldExit(ctx, "runScan") {
			return
		}

		res, err := ddb.ScanWithContext(ctx, &dynamodb.ScanInput{
			TableName:        aws.String(TableName),
			Limit:            aws.Int64(int64(limit)),
			FilterExpression: aws.String("begins_with(#name, :prefix)"),
			ExpressionAttributeNames: map[string]*string{
				"#name": aws.String("id"),
			},
			ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
				":prefix": {
					S: aws.String(fmt.Sprintf("ID:%d", limit-1)),
				},
			},
			ExclusiveStartKey: exclusiveStartKey,
		})

		if err != nil {
			log.Printf("runScan(): scan items: %v", err)
			return
		}

		for _, item := range res.Items {
			user, err := util.UnmarshalWithMetrics[model.User](ctx, item, sender, dynamodbattribute.UnmarshalMap)
			if err != nil {
				log.Printf("runScan(): unmarshal item: %v", err)
				continue
			}

			log.Printf("scanned user %s", user.ID)
		}

		exclusiveStartKey = res.LastEvaluatedKey
		if len(exclusiveStartKey) == 0 {
			break
		}
	}
}

func runUpdate(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runUpdate: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		userRes, err := ddb.GetItemWithContext(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(TableName),
			Key: map[string]*dynamodb.AttributeValue{
				"id": {
					S: aws.String(fmt.Sprintf("ID:%d", l)),
				},
			},
		})
		if err != nil {
			log.Printf("runUpdate(): get item %d: %v", l, err)
			continue
		}

		user, err := util.UnmarshalWithMetrics[model.User](ctx, userRes.Item, sender, dynamodbattribute.UnmarshalMap)
		if err != nil {
			log.Printf("runUpdate(): unmarshal item %d: %v", l, err)
			continue
		}

		user.LoginCount += 1
		user.LoginIPs = append(user.LoginIPs, net.IPv4(byte(l%254+1), byte(l%255), byte(l%255), byte(l%255)).String())
		user.LastLoginAt = aws.Time(time.Now())

		userMap, err := util.MarshalWithMetrics(ctx, &user, sender, dynamodbattribute.MarshalMap)
		if err != nil {
			log.Printf("runUpdate(): marshal user %d: %v", l, err)
			continue
		}

		userMap["id"] = &dynamodb.AttributeValue{
			S: aws.String(fmt.Sprintf("ID:%d", l)),
		}
		_, err = ddb.UpdateItemWithContext(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(TableName),
			Key: map[string]*dynamodb.AttributeValue{
				"id": {
					S: aws.String(fmt.Sprintf("ID:%d", l)),
				},
			},
			UpdateExpression: aws.String("SET login_count = :lc, login_ips = :li, last_login_at = :lla"),
			ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
				":lc": {
					N: aws.String(fmt.Sprintf("%d", user.LoginCount)),
				},
				":li": {
					SS: aws.StringSlice(user.LoginIPs),
				},
				":lla": {
					S: aws.String(user.LastLoginAt.Format(time.RFC3339)),
				},
			},
		})

		if err != nil {
			log.Printf("runUpdate(): update item %d: %v", l, err)
		}
	}
}

func runDelete(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runDelete: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		userMap, err := ddb.GetItemWithContext(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(TableName),
			Key: map[string]*dynamodb.AttributeValue{
				"id": {
					S: aws.String(fmt.Sprintf("ID:%d", l)),
				},
			},
		})
		if err != nil {
			log.Printf("runDelete(): get item %d: %v", l, err)
			continue
		}

		user, err := util.UnmarshalWithMetrics[model.User](ctx, userMap.Item, sender, dynamodbattribute.UnmarshalMap)
		if err != nil {
			log.Printf("runDelete(): unmarshal item %d: %v", l, err)
			continue
		}

		log.Printf("deleting user %d: %+v", l, user)

		_, err = ddb.DeleteItemWithContext(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(TableName),
			Key: map[string]*dynamodb.AttributeValue{
				"id": {
					S: aws.String(user.ID.ID),
				},
			},
		})

		if err != nil {
			log.Printf("runDelete(): delete item %d: %v", l, err)
		}

		log.Printf("deleted user %d", l)
	}
}

func runBatchWrite(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	users := make([]model.User, 0, limit)
	for l := 0; l < limit; l++ {
		user := util.GenerateUser(size)
		user.ID.ID = fmt.Sprintf("ID:%d", l)
		users = append(users, user)
	}

	batchWrite := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]*dynamodb.WriteRequest{
			TableName: {},
		},
	}

	for len(users) > 0 {
		if shouldExit(ctx, "runBatchWrite") {
			return
		}

		for len(users) > 0 && len(batchWrite.RequestItems[TableName]) < BatchWriteLimit {
			user := users[0]
			users = users[1:]

			userMap, err := util.MarshalWithMetrics(ctx, &user, sender, dynamodbattribute.MarshalMap)
			if err != nil {
				log.Printf("runBatchWrite(): marshal user %s: %v", user.ID, err)
				continue
			}

			batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], &dynamodb.WriteRequest{
				PutRequest: &dynamodb.PutRequest{
					Item: userMap,
				},
			})
		}

		res, err := ddb.BatchWriteItemWithContext(ctx, batchWrite)
		if err != nil {
			log.Printf("runBatchWrite(): batch write items: %v", err)
		}
		batchWrite.RequestItems[TableName] = batchWrite.RequestItems[TableName][0:0]

		if len(res.UnprocessedItems) > 0 && len(res.UnprocessedItems[TableName]) > 0 {
			for _, req := range res.UnprocessedItems[TableName] {
				if req.PutRequest != nil {
					batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], &dynamodb.WriteRequest{
						PutRequest: req.PutRequest,
					})
				}
			}
		}
	}
}

func runBatchRead(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]*dynamodb.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(fmt.Sprintf("ID:%d", l)),
			},
		})
	}

	batchGet := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]*dynamodb.KeysAndAttributes{
			TableName: &dynamodb.KeysAndAttributes{
				Keys: []map[string]*dynamodb.AttributeValue{},
			},
		},
	}

	for len(keysToGet) > 0 {
		if shouldExit(ctx, "runBatchRead") {
			return
		}

		for len(batchGet.RequestItems[TableName].Keys) < BatchGetLimit {
			batchGet.RequestItems[TableName].Keys = append(batchGet.RequestItems[TableName].Keys, keysToGet[0])
			keysToGet = keysToGet[1:]
			if len(keysToGet) == 0 {
				break
			}
		}

		res, err := ddb.BatchGetItemWithContext(ctx, batchGet)
		if err != nil {
			log.Printf("runBatchRead(): batch get items: %v", err)
		}
		batchGet.RequestItems[TableName].Keys = batchGet.RequestItems[TableName].Keys[0:0]

		for _, item := range res.Responses[TableName] {
			user, err := util.UnmarshalWithMetrics[model.User](ctx, item, sender, dynamodbattribute.UnmarshalMap)
			if err != nil {
				log.Printf("runBatchRead(): unmarshal item: %v", err)
				continue
			}

			log.Printf("batch read user %s", user.ID)
		}

		if len(res.UnprocessedKeys) > 0 && len(res.UnprocessedKeys[TableName].Keys) > 0 {
			for _, key := range res.UnprocessedKeys[TableName].Keys {
				keysToGet = append(keysToGet, key)
			}
		}
	}
}

func runBatchDelete(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	users := make([]model.User, 0, limit)

	keysToGet := make([]map[string]*dynamodb.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(fmt.Sprintf("ID:%d", l)),
			},
		})
	}

	batchGet := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]*dynamodb.KeysAndAttributes{
			TableName: &dynamodb.KeysAndAttributes{
				Keys: []map[string]*dynamodb.AttributeValue{},
			},
		},
	}

	for len(keysToGet) > 0 {
		for len(keysToGet) > 0 && len(batchGet.RequestItems[TableName].Keys) < BatchWriteLimit {
			batchGet.RequestItems[TableName].Keys = append(batchGet.RequestItems[TableName].Keys, keysToGet[0])
			keysToGet = keysToGet[1:]
		}

		res, err := ddb.BatchGetItemWithContext(ctx, batchGet)
		if err != nil {
			log.Printf("runBatchRead(): batch get items: %v", err)
		}
		batchGet.RequestItems[TableName].Keys = batchGet.RequestItems[TableName].Keys[0:0]

		for _, item := range res.Responses[TableName] {
			user, err := util.UnmarshalWithMetrics[model.User](ctx, item, sender, dynamodbattribute.UnmarshalMap)
			if err != nil {
				log.Printf("runBatchRead(): unmarshal item: %v", err)
				continue
			}

			users = append(users, *user)

			log.Printf("batch read user %s", user.ID)
		}

		if len(res.UnprocessedKeys) > 0 && len(res.UnprocessedKeys[TableName].Keys) > 0 {
			for _, key := range res.UnprocessedKeys[TableName].Keys {
				keysToGet = append(keysToGet, key)
			}
		}
	}

	if len(users) == 0 {
		log.Print("no users to delete in batch delete")
	}

	batchWrite := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]*dynamodb.WriteRequest{
			TableName: {},
		},
	}

	for len(users) > 0 {
		if shouldExit(ctx, "runBatchDelete") {
			return
		}

		for len(users) > 0 && len(batchWrite.RequestItems[TableName]) < BatchWriteLimit {
			if shouldExit(ctx, "runBatchDelete") {
				return
			}

			user := users[0]
			users = users[1:]

			userMap, err := util.MarshalWithMetrics(ctx, &user, sender, dynamodbattribute.MarshalMap)
			if err != nil {
				log.Printf("runBatchDelete(): marshal user %s: %v", user.ID, err)
				continue
			}

			batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], &dynamodb.WriteRequest{
				DeleteRequest: &dynamodb.DeleteRequest{
					Key: map[string]*dynamodb.AttributeValue{
						"id": userMap["id"],
					},
				},
			})
		}

		res, err := ddb.BatchWriteItemWithContext(ctx, batchWrite)
		if err != nil {
			log.Printf("runBatchDelete(): batch write items: %v", err)
		}
		batchWrite.RequestItems[TableName] = batchWrite.RequestItems[TableName][0:0]

		if len(res.UnprocessedItems) > 0 && len(res.UnprocessedItems[TableName]) > 0 {
			for _, req := range res.UnprocessedItems[TableName] {
				if req.DeleteRequest != nil {
					batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], &dynamodb.WriteRequest{
						DeleteRequest: req.DeleteRequest,
					})
				}
			}
		}
	}
}

func runTransactPut(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	users := make([]model.User, 0, limit)
	for l := 0; l < limit; l++ {
		user := util.GenerateUser(size)
		user.ID.ID = fmt.Sprintf("ID:%d", l)
		users = append(users, user)
	}

	transactItems := make([]*dynamodb.TransactWriteItem, 0, limit)
	for _, user := range users {
		userMap, err := util.MarshalWithMetrics(ctx, &user, sender, dynamodbattribute.MarshalMap)
		if err != nil {
			log.Printf("runTransactPut(): marshal user %s: %v", user.ID, err)
			continue
		}

		transactItems = append(transactItems, &dynamodb.TransactWriteItem{
			Put: &dynamodb.Put{
				TableName: aws.String(TableName),
				Item:      userMap,
			},
		})
	}

	for len(transactItems) > 0 {
		if shouldExit(ctx, "runTransactPut") {
			return
		}

		batch := transactItems
		if len(batch) > 0 {
			batch = batch[:util.Min(TransactWriteLimit, len(batch))]
			transactItems = transactItems[util.Min(TransactWriteLimit, len(transactItems)):]
		}

		_, err := ddb.TransactWriteItemsWithContext(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: batch,
		})
		if err != nil {
			log.Printf("runTransactPut(): transact write items: %v", err)
		} else {
			log.Printf("transact put %d items", len(batch))
		}
	}
}

func runTransactGet(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]*dynamodb.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(fmt.Sprintf("ID:%d", l)),
			},
		})
	}

	for len(keysToGet) > 0 {
		if shouldExit(ctx, "runTransactGet") {
			return
		}

		batch := keysToGet
		if len(batch) > 0 {
			batch = batch[:util.Min(TransactGetLimit, len(batch))]
			keysToGet = keysToGet[util.Min(TransactGetLimit, len(keysToGet)):]
		}

		res, err := ddb.TransactGetItemsWithContext(ctx, &dynamodb.TransactGetItemsInput{
			TransactItems: func() []*dynamodb.TransactGetItem {
				out := make([]*dynamodb.TransactGetItem, 0, len(batch))
				for _, key := range batch {
					out = append(out, &dynamodb.TransactGetItem{
						Get: &dynamodb.Get{
							TableName: aws.String(TableName),
							Key:       key,
						},
					})
				}
				return out
			}(),
		})
		if err != nil {
			log.Printf("runTransactGet(): transact get items: %v", err)
			continue
		}

		for _, item := range res.Responses {
			user, err := util.UnmarshalWithMetrics[model.User](ctx, item.Item, sender, dynamodbattribute.UnmarshalMap)
			if err != nil {
				log.Printf("runTransactGet(): unmarshal item: %v", err)
				continue
			}

			log.Printf("transact got user %s", user.ID)
		}
	}
}

func runTransactUpdate(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]*dynamodb.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(fmt.Sprintf("ID:%d", l)),
			},
		})
	}

	for len(keysToGet) > 0 {
		if shouldExit(ctx, "runTransactUpdate") {
			return
		}

		batch := keysToGet
		if len(batch) > 0 {
			batch = batch[:util.Min(TransactGetLimit, len(batch))]
			keysToGet = keysToGet[util.Min(TransactGetLimit, len(keysToGet)):]
		}

		res, err := ddb.TransactGetItemsWithContext(ctx, &dynamodb.TransactGetItemsInput{
			TransactItems: func() []*dynamodb.TransactGetItem {
				out := make([]*dynamodb.TransactGetItem, 0, len(batch))
				for _, key := range batch {
					out = append(out, &dynamodb.TransactGetItem{
						Get: &dynamodb.Get{
							TableName: aws.String(TableName),
							Key:       key,
						},
					})
				}
				return out
			}(),
		})
		if err != nil {
			log.Printf("runTransactUpdate(): transact get items: %v", err)
			continue
		}

		transactItems := make([]*dynamodb.TransactWriteItem, 0, len(res.Responses))
		for _, item := range res.Responses {
			user, err := util.UnmarshalWithMetrics[model.User](ctx, item.Item, sender, dynamodbattribute.UnmarshalMap)
			if err != nil {
				log.Printf("runTransactUpdate(): unmarshal item: %v", err)
				continue
			}

			user.LoginCount += 1
			user.LoginIPs = append(user.LoginIPs, net.IPv4(byte(user.LoginCount%254+1), byte(user.LoginCount%255), byte(user.LoginCount%255), byte(user.LoginCount%255)).String())
			user.LastLoginAt = aws.Time(time.Now())

			userMap, err := util.MarshalWithMetrics(ctx, user, sender, dynamodbattribute.MarshalMap)
			if err != nil {
				log.Printf("runTransactUpdate(): marshal user %s: %v", user.ID, err)
				continue
			}

			transactItems = append(transactItems, &dynamodb.TransactWriteItem{
				Update: &dynamodb.Update{
					TableName: aws.String(TableName),
					Key: map[string]*dynamodb.AttributeValue{
						"id": userMap["id"],
					},
					UpdateExpression: aws.String("SET login_count = :lc, login_ips = :li, last_login_at = :lla"),
					ExpressionAttributeValues: map[string]*dynamodb.AttributeValue{
						":lc": {
							N: aws.String(fmt.Sprintf("%d", user.LoginCount)),
						},
						":li": {
							SS: aws.StringSlice(user.LoginIPs),
						},
						":lla": {
							S: aws.String(user.LastLoginAt.Format(time.RFC3339)),
						},
					},
				},
			})
		}

		for len(transactItems) > 0 {
			if shouldExit(ctx, "runTransactUpdate") {
				return
			}

			batch := transactItems
			if len(batch) > 0 {
				batch = batch[:util.Min(TransactWriteLimit, len(batch))]
				transactItems = transactItems[util.Min(TransactWriteLimit, len(transactItems)):]
			}

			_, err := ddb.TransactWriteItemsWithContext(ctx, &dynamodb.TransactWriteItemsInput{
				TransactItems: batch,
			})
			if err != nil {
				log.Printf("runTransactUpdate(): transact write items: %v", err)
			} else {
				log.Printf("transact updated %d items", len(batch))
			}
		}
	}
}

func runTransactDelete(ctx context.Context, size util.Size, limit int, ddb *dynamodb.DynamoDB, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]*dynamodb.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]*dynamodb.AttributeValue{
			"id": {
				S: aws.String(fmt.Sprintf("ID:%d", l)),
			},
		})
	}

	for len(keysToGet) > 0 {
		if shouldExit(ctx, "runTransactDelete") {
			return
		}

		batch := keysToGet
		if len(batch) > 0 {
			batch = batch[:util.Min(TransactGetLimit, len(batch))]
			keysToGet = keysToGet[util.Min(TransactGetLimit, len(keysToGet)):]
		}

		res, err := ddb.TransactGetItemsWithContext(ctx, &dynamodb.TransactGetItemsInput{
			TransactItems: func() []*dynamodb.TransactGetItem {
				out := make([]*dynamodb.TransactGetItem, 0, len(batch))
				for _, key := range batch {
					out = append(out, &dynamodb.TransactGetItem{
						Get: &dynamodb.Get{
							TableName: aws.String(TableName),
							Key:       key,
						},
					})
				}
				return out
			}(),
		})
		if err != nil {
			log.Printf("runTransactDelete(): transact get items: %v", err)
			continue
		}

		transactItems := make([]*dynamodb.TransactWriteItem, 0, len(res.Responses))
		for _, item := range res.Responses {
			user, err := util.UnmarshalWithMetrics[model.User](ctx, item.Item, sender, dynamodbattribute.UnmarshalMap)
			if err != nil {
				log.Printf("runTransactDelete(): unmarshal item: %v", err)
				continue
			}

			userMap, err := util.MarshalWithMetrics(ctx, user, sender, dynamodbattribute.MarshalMap)
			if err != nil {
				log.Printf("runTransactDelete(): marshal user %s: %v", user.ID, err)
				continue
			}

			transactItems = append(transactItems, &dynamodb.TransactWriteItem{
				Delete: &dynamodb.Delete{
					TableName: aws.String(TableName),
					Key: map[string]*dynamodb.AttributeValue{
						"id": userMap["id"],
					},
				},
			})
		}

		for len(transactItems) > 0 {
			if shouldExit(ctx, "runTransactDelete") {
				return
			}

			batch := transactItems
			if len(batch) > 0 {
				batch = batch[:util.Min(TransactWriteLimit, len(batch))]
				transactItems = transactItems[util.Min(TransactWriteLimit, len(transactItems)):]
			}

			_, err := ddb.TransactWriteItemsWithContext(ctx, &dynamodb.TransactWriteItemsInput{
				TransactItems: batch,
			})
			if err != nil {
				log.Printf("runTransactDelete(): transact write items: %v", err)
			} else {
				log.Printf("transact deleted %d items", len(batch))
			}
		}
	}
}

func shouldExit(ctx context.Context, fnName string) bool {
	select {
	case <-ctx.Done():
		log.Printf("shutting down %s: %v", fnName, ctx.Err())
		return true
	default:
		return false
	}
}

func awsSession() *session.Session {
	cfg := &aws.Config{}
	cfg.WithRegion(util.Env("AWS_REGION", "eu-west-1"))

	if endpoint := util.Env("AWS_ENDPOINT_URL", ""); endpoint != "" {
		cfg.WithEndpoint(util.Env("AWS_ENDPOINT_URL", ""))

		sc := credentials.NewStaticCredentials("test", "test", "test")
		cfg.WithCredentials(sc)
	} else if util.Env("AWS_ACCESS_KEY_ID", "") != "" && util.Env("AWS_SECRET_ACCESS_KEY", "") != "" {
		sc := credentials.NewChainCredentials(
			[]credentials.Provider{
				&credentials.SharedCredentialsProvider{
					Filename: util.Env("AWS_SHARED_CREDENTIALS_FILE", "~/.aws/config"),
					Profile:  util.Env("AWS_PROFILE", "default"),
				},
				&credentials.EnvProvider{},
			},
		)

		println("Using credentials chain: SharedCredentialsProvider -> EnvProvider")
		println("File:", util.Env("AWS_SHARED_CREDENTIALS_FILE", "~/.aws/config"))
		println("Profile:", util.Env("AWS_PROFILE", "default"))
		cfg.WithCredentials(sc)
	}

	return session.Must(session.NewSessionWithOptions(session.Options{
		Config:            *cfg,
		Profile:           util.Env("AWS_PROFILE", "default"),
		SharedConfigState: session.SharedConfigDisable,
	}))
}

func dynamoDBService(sess *session.Session) *dynamodb.DynamoDB {
	return dynamodb.New(sess)
}

func ensureTable(ddb *dynamodb.DynamoDB) {
	desc, err := ddb.DescribeTable(&dynamodb.DescribeTableInput{
		TableName: aws.String(TableName),
	})

	if desc == nil || desc.Table == nil {
		_, err = ddb.CreateTable(&dynamodb.CreateTableInput{
			TableName: aws.String(TableName),
			AttributeDefinitions: []*dynamodb.AttributeDefinition{
				{
					AttributeName: aws.String("id"),
					AttributeType: aws.String("S"),
				},
			},
			KeySchema: []*dynamodb.KeySchemaElement{
				{
					AttributeName: aws.String("id"),
					KeyType:       aws.String("HASH"),
				},
			},
			BillingMode: aws.String("PAY_PER_REQUEST"),
		})

		if err != nil {
			panic(err)
		}

		_ = ddb.WaitUntilTableExists(&dynamodb.DescribeTableInput{
			TableName: aws.String(TableName),
		})
	} else if err != nil {
		panic(err)
	}
}

func cloudwatchService(sess *session.Session) *cloudwatch.CloudWatch {
	return cloudwatch.New(sess)
}
