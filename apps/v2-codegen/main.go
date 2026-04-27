package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os/signal"
	"pkg/model"
	"pkg/util"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/smithy-go/metrics"
)

const TableName = "test_table_benchmarks_v2_codegen_" + runtime.GOARCH
const CloudWatchNamespace = "Benchmark/GoSDKv2CodeGen"
const BatchGetLimit = 25
const BatchWriteLimit = 25
const TransactGetLimit = 100
const TransactWriteLimit = 100

func main() {
	size := util.Size(util.Env("SIZE", "1KB"))
	if !size.Valid() {
		panic(fmt.Sprintf("invalid SIZE: %v, valid values: %v", size, size.Values()))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGHUP)
	defer stop()

	sess := awsSession()
	cw := cloudwatchService(sess)

	sender := &util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]{
		Namespace: CloudWatchNamespace,
		Queue:     make(chan model.Event, 1024*1024),
		Sender: func(input *cloudwatch.PutMetricDataInput) (*cloudwatch.PutMetricDataOutput, error) {
			// use a fresh context to avoid the canceled context from mainDynamoDB affecting the CloudWatch calls
			return cw.PutMetricData(context.Background(), input)
		},
		Dimensions: []model.Dimension{
			{Name: util.Pointer(util.DimensionNameSDK), Value: util.Pointer(util.DimensionSDKV2)},
			{Name: util.Pointer(util.DimensionNameSize), Value: util.Pointer(string(size))},
			{Name: util.Pointer(util.DimensionNameOS), Value: util.Pointer(runtime.GOOS)},
			{Name: util.Pointer(util.DimensionNameArch), Value: util.Pointer(runtime.GOARCH)},
		},
	}

	ddb := dynamoDBService(sess, sender)

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
	defer wg.Done()

	//
}

func mainDynamoDB(ctx context.Context, wg *sync.WaitGroup, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput], stop context.CancelFunc, size util.Size) {
	defer func() {
		wg.Done()
		log.Println("mainDynamoDB exiting")
	}()
	defer stop()

	ddbWg := &sync.WaitGroup{}

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

func runPut(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runPut: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		user := GenerateUser(size)

		userMap, err := util.MarshalWithMetrics(ctx, &user, sender, attributevalue.MarshalMap)
		if err != nil {
			log.Printf("runPut(): marshal user %d: %v", l, err)
			continue
		}

		userMap["id"] = &types.AttributeValueMemberS{
			Value: fmt.Sprintf("ID:%d", l),
		}
		_, err = ddb.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: aws.String(TableName),
			Item:      userMap,
		})

		if err != nil {
			log.Printf("runPut(): put item %d: %v", l, err)
		}
	}
}

func runRead(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runRead: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		res, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(TableName),
			Key: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{
					Value: fmt.Sprintf("ID:%d", l),
				},
			},
		})
		if err != nil {
			log.Printf("runRead(): get item %d: %v", l, err)
			continue
		}

		user, err := util.UnmarshalWithMetrics[User](ctx, res.Item, sender, attributevalue.UnmarshalMap)
		if err != nil {
			log.Printf("runRead(): unmarshal item %d: %v", l, err)
			continue
		}

		log.Printf("read user %s", user.ID)
	}
}

func runQuery(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		var exclusiveStartKey map[string]types.AttributeValue

		for {
			if shouldExit(ctx, "runQuery") {
				return
			}

			res, err := ddb.Query(ctx, &dynamodb.QueryInput{
				TableName:              aws.String(TableName),
				KeyConditionExpression: aws.String("id = :id"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":id": &types.AttributeValueMemberS{
						Value: fmt.Sprintf("ID:%d", l),
					},
				},
				ExclusiveStartKey: exclusiveStartKey,
			})

			if err != nil {
				log.Printf("runQuery(): query item %d: %v", l, err)
				continue
			}

			for _, item := range res.Items {
				user, err := util.UnmarshalWithMetrics[User](ctx, item, sender, attributevalue.UnmarshalMap)
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

func runScan(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	var exclusiveStartKey map[string]types.AttributeValue

	for {
		if shouldExit(ctx, "runScan") {
			return
		}

		res, err := ddb.Scan(ctx, &dynamodb.ScanInput{
			TableName:        aws.String(TableName),
			Limit:            aws.Int32(int32(limit)),
			FilterExpression: aws.String("begins_with(#name, :prefix)"),
			ExpressionAttributeNames: map[string]string{
				"#name": "id",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":prefix": &types.AttributeValueMemberS{
					Value: fmt.Sprintf("ID:%d", limit-1),
				},
			},
			ExclusiveStartKey: exclusiveStartKey,
		})

		if err != nil {
			log.Printf("runScan(): scan items: %v", err)
			return
		}

		for _, item := range res.Items {
			user, err := util.UnmarshalWithMetrics[User](ctx, item, sender, attributevalue.UnmarshalMap)
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

func runUpdate(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runUpdate: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		userRes, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(TableName),
			Key: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{
					Value: fmt.Sprintf("ID:%d", l),
				},
			},
		})
		if err != nil {
			log.Printf("runUpdate(): get item %d: %v", l, err)
			continue
		}

		user, err := util.UnmarshalWithMetrics[User](ctx, userRes.Item, sender, attributevalue.UnmarshalMap)
		if err != nil {
			log.Printf("runUpdate(): unmarshal item %d: %v", l, err)
			continue
		}

		user.LoginCount += 1
		user.LoginIPs = append(user.LoginIPs, net.IPv4(byte(l%254+1), byte(l%255), byte(l%255), byte(l%255)).String())
		user.LastLoginAt = aws.Time(time.Now())

		userMap, err := util.MarshalWithMetrics(ctx, &user, sender, attributevalue.MarshalMap)
		if err != nil {
			log.Printf("runUpdate(): marshal user %d: %v", l, err)
			continue
		}

		userMap["id"] = &types.AttributeValueMemberS{
			Value: fmt.Sprintf("ID:%d", l),
		}
		_, err = ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
			TableName: aws.String(TableName),
			Key: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{
					Value: fmt.Sprintf("ID:%d", l),
				},
			},
			UpdateExpression: aws.String("SET login_count = :lc, login_ips = :li, last_login_at = :lla"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":lc": &types.AttributeValueMemberN{
					Value: fmt.Sprintf("%d", user.LoginCount),
				},
				":li": &types.AttributeValueMemberSS{
					Value: user.LoginIPs,
				},
				":lla": &types.AttributeValueMemberS{
					Value: user.LastLoginAt.Format(time.RFC3339),
				},
			},
		})

		if err != nil {
			log.Printf("runUpdate(): update item %d: %v", l, err)
		}
	}
}

func runDelete(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	for l := 0; l < limit; l++ {
		select {
		case <-ctx.Done():
			log.Printf("shutting down runDelete: %v", ctx.Err())
			return
		default:
			//no-op
			break
		}

		userMap, err := ddb.GetItem(ctx, &dynamodb.GetItemInput{
			TableName: aws.String(TableName),
			Key: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{
					Value: fmt.Sprintf("ID:%d", l),
				},
			},
		})
		if err != nil {
			log.Printf("runDelete(): get item %d: %v", l, err)
			continue
		}

		user, err := util.UnmarshalWithMetrics[User](ctx, userMap.Item, sender, attributevalue.UnmarshalMap)
		if err != nil {
			log.Printf("runDelete(): unmarshal item %d: %v", l, err)
			continue
		}

		log.Printf("deleting user %d: %+v", l, user)

		_, err = ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(TableName),
			Key: map[string]types.AttributeValue{
				"id": &types.AttributeValueMemberS{
					Value: user.ID.ID,
				},
			},
		})

		if err != nil {
			log.Printf("runDelete(): delete item %d: %v", l, err)
		}

		log.Printf("deleted user %d", l)
	}
}

func runBatchWrite(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	users := make([]User, 0, limit)
	for l := 0; l < limit; l++ {
		user := GenerateUser(size)
		user.ID.ID = fmt.Sprintf("ID:%d", l)
		users = append(users, user)
	}

	batchWrite := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
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

			userMap, err := util.MarshalWithMetrics(ctx, &user, sender, attributevalue.MarshalMap)
			if err != nil {
				log.Printf("runBatchWrite(): marshal user %s: %v", user.ID, err)
				continue
			}

			batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], types.WriteRequest{
				PutRequest: &types.PutRequest{
					Item: userMap,
				},
			})
		}

		res, err := ddb.BatchWriteItem(ctx, batchWrite)
		if err != nil {
			log.Printf("runBatchWrite(): batch write items: %v", err)
		}
		batchWrite.RequestItems[TableName] = batchWrite.RequestItems[TableName][0:0]

		if len(res.UnprocessedItems) > 0 && len(res.UnprocessedItems[TableName]) > 0 {
			for _, req := range res.UnprocessedItems[TableName] {
				if req.PutRequest != nil {
					batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], types.WriteRequest{
						PutRequest: req.PutRequest,
					})
				}
			}
		}
	}
}

func runBatchRead(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]types.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("ID:%d", l),
			},
		})
	}

	batchGet := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			TableName: types.KeysAndAttributes{
				Keys: []map[string]types.AttributeValue{},
			},
		},
	}

	for len(keysToGet) > 0 {
		if shouldExit(ctx, "runBatchRead") {
			return
		}

		for len(batchGet.RequestItems[TableName].Keys) < BatchGetLimit {
			kaa := batchGet.RequestItems[TableName]
			kaa.Keys = append(batchGet.RequestItems[TableName].Keys, keysToGet[0])
			batchGet.RequestItems[TableName] = kaa

			keysToGet = keysToGet[1:]
			if len(keysToGet) == 0 {
				break
			}
		}

		res, err := ddb.BatchGetItem(ctx, batchGet)
		if err != nil {
			log.Printf("runBatchRead(): batch get items: %v", err)
		}
		kaa := batchGet.RequestItems[TableName]
		kaa.Keys = batchGet.RequestItems[TableName].Keys[0:0]
		batchGet.RequestItems[TableName] = kaa

		for _, item := range res.Responses[TableName] {
			user, err := util.UnmarshalWithMetrics[User](ctx, item, sender, attributevalue.UnmarshalMap)
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

func runBatchDelete(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	users := make([]User, 0, limit)

	keysToGet := make([]map[string]types.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("ID:%d", l),
			},
		})
	}

	batchGet := &dynamodb.BatchGetItemInput{
		RequestItems: map[string]types.KeysAndAttributes{
			TableName: types.KeysAndAttributes{
				Keys: []map[string]types.AttributeValue{},
			},
		},
	}

	for len(keysToGet) > 0 {
		for len(keysToGet) > 0 && len(batchGet.RequestItems[TableName].Keys) < BatchWriteLimit {
			kaa := batchGet.RequestItems[TableName]
			kaa.Keys = append(batchGet.RequestItems[TableName].Keys, keysToGet[0])
			batchGet.RequestItems[TableName] = kaa

			keysToGet = keysToGet[1:]
		}

		res, err := ddb.BatchGetItem(ctx, batchGet)
		if err != nil {
			log.Printf("runBatchRead(): batch get items: %v", err)
		}
		kaa := batchGet.RequestItems[TableName]
		kaa.Keys = batchGet.RequestItems[TableName].Keys[0:0]
		batchGet.RequestItems[TableName] = kaa

		for _, item := range res.Responses[TableName] {
			user, err := util.UnmarshalWithMetrics[User](ctx, item, sender, attributevalue.UnmarshalMap)
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
		RequestItems: map[string][]types.WriteRequest{
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

			userMap, err := util.MarshalWithMetrics(ctx, &user, sender, attributevalue.MarshalMap)
			if err != nil {
				log.Printf("runBatchDelete(): marshal user %s: %v", user.ID, err)
				continue
			}

			batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"id": userMap["id"],
					},
				},
			})
		}

		res, err := ddb.BatchWriteItem(ctx, batchWrite)
		if err != nil {
			log.Printf("runBatchDelete(): batch write items: %v", err)
		}
		batchWrite.RequestItems[TableName] = batchWrite.RequestItems[TableName][0:0]

		if len(res.UnprocessedItems) > 0 && len(res.UnprocessedItems[TableName]) > 0 {
			for _, req := range res.UnprocessedItems[TableName] {
				if req.DeleteRequest != nil {
					batchWrite.RequestItems[TableName] = append(batchWrite.RequestItems[TableName], types.WriteRequest{
						DeleteRequest: req.DeleteRequest,
					})
				}
			}
		}
	}
}

func runTransactPut(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	users := make([]User, 0, limit)
	for l := 0; l < limit; l++ {
		user := GenerateUser(size)
		user.ID.ID = fmt.Sprintf("ID:%d", l)
		users = append(users, user)
	}

	transactItems := make([]types.TransactWriteItem, 0, limit)
	for _, user := range users {
		userMap, err := util.MarshalWithMetrics(ctx, &user, sender, attributevalue.MarshalMap)
		if err != nil {
			log.Printf("runTransactPut(): marshal user %s: %v", user.ID, err)
			continue
		}

		transactItems = append(transactItems, types.TransactWriteItem{
			Put: &types.Put{
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

		_, err := ddb.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
			TransactItems: batch,
		})
		if err != nil {
			log.Printf("runTransactPut(): transact write items: %v", err)
		} else {
			log.Printf("transact put %d items", len(batch))
		}
	}
}

func runTransactGet(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]types.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("ID:%d", l),
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

		res, err := ddb.TransactGetItems(ctx, &dynamodb.TransactGetItemsInput{
			TransactItems: func() []types.TransactGetItem {
				out := make([]types.TransactGetItem, 0, len(batch))
				for _, key := range batch {
					out = append(out, types.TransactGetItem{
						Get: &types.Get{
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
			user, err := util.UnmarshalWithMetrics[User](ctx, item.Item, sender, attributevalue.UnmarshalMap)
			if err != nil {
				log.Printf("runTransactGet(): unmarshal item: %v", err)
				continue
			}

			log.Printf("transact got user %s", user.ID)
		}
	}
}

func runTransactUpdate(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]types.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("ID:%d", l),
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

		res, err := ddb.TransactGetItems(ctx, &dynamodb.TransactGetItemsInput{
			TransactItems: func() []types.TransactGetItem {
				out := make([]types.TransactGetItem, 0, len(batch))
				for _, key := range batch {
					out = append(out, types.TransactGetItem{
						Get: &types.Get{
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

		transactItems := make([]types.TransactWriteItem, 0, len(res.Responses))
		for _, item := range res.Responses {
			user, err := util.UnmarshalWithMetrics[User](ctx, item.Item, sender, attributevalue.UnmarshalMap)
			if err != nil {
				log.Printf("runTransactUpdate(): unmarshal item: %v", err)
				continue
			}

			user.LoginCount += 1
			user.LoginIPs = append(user.LoginIPs, net.IPv4(byte(user.LoginCount%254+1), byte(user.LoginCount%255), byte(user.LoginCount%255), byte(user.LoginCount%255)).String())
			user.LastLoginAt = aws.Time(time.Now())

			userMap, err := util.MarshalWithMetrics(ctx, user, sender, attributevalue.MarshalMap)
			if err != nil {
				log.Printf("runTransactUpdate(): marshal user %s: %v", user.ID, err)
				continue
			}

			transactItems = append(transactItems, types.TransactWriteItem{
				Update: &types.Update{
					TableName: aws.String(TableName),
					Key: map[string]types.AttributeValue{
						"id": userMap["id"],
					},
					UpdateExpression: aws.String("SET login_count = :lc, login_ips = :li, last_login_at = :lla"),
					ExpressionAttributeValues: map[string]types.AttributeValue{
						":lc": &types.AttributeValueMemberN{
							Value: fmt.Sprintf("%d", user.LoginCount),
						},
						":li": &types.AttributeValueMemberSS{
							Value: user.LoginIPs,
						},
						":lla": &types.AttributeValueMemberS{
							Value: user.LastLoginAt.Format(time.RFC3339),
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

			_, err := ddb.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
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

func runTransactDelete(ctx context.Context, size util.Size, limit int, ddb *dynamodb.Client, sender *util.CloudWatchSender[cloudwatch.PutMetricDataInput, cloudwatch.PutMetricDataOutput]) {
	keysToGet := make([]map[string]types.AttributeValue, 0, limit)
	for l := 0; l < limit; l++ {
		keysToGet = append(keysToGet, map[string]types.AttributeValue{
			"id": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("ID:%d", l),
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

		res, err := ddb.TransactGetItems(ctx, &dynamodb.TransactGetItemsInput{
			TransactItems: func() []types.TransactGetItem {
				out := make([]types.TransactGetItem, 0, len(batch))
				for _, key := range batch {
					out = append(out, types.TransactGetItem{
						Get: &types.Get{
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

		transactItems := make([]types.TransactWriteItem, 0, len(res.Responses))
		for _, item := range res.Responses {
			user, err := util.UnmarshalWithMetrics[User](ctx, item.Item, sender, attributevalue.UnmarshalMap)
			if err != nil {
				log.Printf("runTransactDelete(): unmarshal item: %v", err)
				continue
			}

			userMap, err := util.MarshalWithMetrics(ctx, user, sender, attributevalue.MarshalMap)
			if err != nil {
				log.Printf("runTransactDelete(): marshal user %s: %v", user.ID, err)
				continue
			}

			transactItems = append(transactItems, types.TransactWriteItem{
				Delete: &types.Delete{
					TableName: aws.String(TableName),
					Key: map[string]types.AttributeValue{
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

			_, err := ddb.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
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

func cloudwatchService(cfg aws.Config) *cloudwatch.Client {
	return cloudwatch.NewFromConfig(cfg)
}

func dynamoDBService(cfg aws.Config, mp metrics.MeterProvider) *dynamodb.Client {
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.MeterProvider = mp
	})
}

func ensureTable(ddb *dynamodb.Client) {
	desc, err := ddb.DescribeTable(context.Background(), &dynamodb.DescribeTableInput{
		TableName: aws.String(TableName),
	})

	if desc == nil || desc.Table == nil {
		_, err = ddb.CreateTable(context.Background(), &dynamodb.CreateTableInput{
			TableName: aws.String(TableName),
			AttributeDefinitions: []types.AttributeDefinition{
				{
					AttributeName: aws.String("id"),
					AttributeType: types.ScalarAttributeTypeS,
				},
			},
			KeySchema: []types.KeySchemaElement{
				{
					AttributeName: aws.String("id"),
					KeyType:       types.KeyTypeHash,
				},
			},
			BillingMode: types.BillingModePayPerRequest,
		})

		if err != nil {
			panic(err)
		}

		err = dynamodb.NewTableExistsWaiter(ddb).Wait(context.Background(), &dynamodb.DescribeTableInput{
			TableName: aws.String(TableName),
		}, 5*time.Minute)
		if err != nil {
			panic(err)
		}
	} else if err != nil {
		panic(err)
	}
}

func awsSession() aws.Config {
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion(util.Env("AWS_REGION", "eu-west-1")),
	)

	if err != nil {
		panic(err)
	}

	return cfg
}
