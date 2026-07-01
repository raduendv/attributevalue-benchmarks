package util

import (
	"context"
	"fmt"
	"log"
	"pkg/model"
	"sync"
	"time"

	v2 "github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	v1 "github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/smithy-go/metrics"
)

type PutMetricDataInput interface {
	v1.PutMetricDataInput | v2.PutMetricDataInput
}

type PutMetricDataOutput interface {
	v1.PutMetricDataOutput | v2.PutMetricDataOutput
}

type Sender[I PutMetricDataInput, O PutMetricDataOutput] func(*I) (*O, error)

type CloudWatchSender[I PutMetricDataInput, O PutMetricDataOutput] struct {
	Namespace  string
	Dimensions []model.Dimension
	Queue      chan model.Event
	Sender     Sender[I, O]

	Mutex  sync.Mutex
	Meters map[string]metrics.Meter
}

func (s *CloudWatchSender[I, O]) Send(event model.Event) {
	if event.API != nil && *event.API == "PutMetricData" {
		log.Printf("CloudWatchSender dropping event for PutMetricData to avoid recursion: %s\n", *event.API)
		return
	}

	select {
	case s.Queue <- event:
		log.Printf("CloudWatchSender queued event: %s\n", *event.API)
	default:
		log.Printf("CloudWatchSender queue full, dropping event: %s\n", *event.API)
	}
}

func (s *CloudWatchSender[I, O]) flush(all bool) {
	log.Println("CloudWatchSender flush called")

	for {
		count := len(s.Queue)
		if count == 0 {
			log.Println("CloudWatchSender flush: no events to send")
			return
		}

		es := make([]model.Event, 0, 500)
		for i := 0; i < count && i < 500; i++ {
			select {
			case event := <-s.Queue:
				es = append(es, event)
			case <-time.After(time.Second * 5):
				break
			}
		}

		input, err := makeInput[I](s.Namespace, s.Dimensions, es)
		if err != nil {
			log.Printf("CloudWatchSender error creating input: %v\n", err)
			return
		}

		_, err = s.Sender(input)
		if err != nil {
			log.Printf("CloudWatchSender error sending metrics: %v\n", err)
		} else {
			log.Printf("CloudWatchSender successfully sent %d events\n", len(es))
		}

		if !all {
			return
		}
	}
}

func (s *CloudWatchSender[I, O]) senderGoroutine(ctx context.Context) {
	log.Println("CloudWatchSender sender started")
	defer log.Println("CloudWatchSender sender exiting")

	toSend := make([]model.Event, 0, 500)

	for {
		select {
		case <-ctx.Done():
			log.Println("CloudWatchSender sender received shutdown signal")
			for _, event := range toSend {
				log.Printf("Requeueing event due to shutdown: %s\n", *event.API)
				s.Send(event)
			}
			return
		case event := <-s.Queue:
			toSend = append(toSend, event)
			if len(toSend) >= 500 {
				input, err := makeInput[I](s.Namespace, s.Dimensions, toSend)
				if err != nil {
					log.Printf("CloudWatchSender error creating input: %v\n", err)
					toSend = toSend[:0]
					continue
				}

				_, err = s.Sender(input)
				if err != nil {
					log.Printf("CloudWatchSender error sending metrics: %v\n", err)
				} else {
					log.Printf("CloudWatchSender successfully sent %d events\n", len(toSend))
				}
				toSend = toSend[:0]
			}
		}
	}
}

func (s *CloudWatchSender[I, O]) Start(ctx context.Context, wg *sync.WaitGroup) {
	defer func() {
		wg.Done()
		log.Println("CloudWatchSender exiting")
	}()

	var cancels []context.CancelFunc

	// start the collectors
	log.Println("CloudWatchSender starting collectors")
	go CollectGCStats(ctx, s.Send)
	go CollectMemoryStats(ctx, s.Send)
	go CollectRuntimeStats(ctx, s.Send)
	go CollectCPUStats(ctx, s.Send)

	for {
		select {
		case <-time.After(time.Second):
			c := len(s.Queue)
			if c > 500 {
				toStart := (c / 500) + 1 - len(cancels)
				log.Printf("CloudWatchSender starting %d senders to flush %d events\n", toStart, c)
				for i := 0; i < toStart; i++ {
					cctx, cancel := context.WithTimeout(ctx, time.Second*30)
					go s.senderGoroutine(cctx)
					cancels = append(cancels, cancel)
				}
			} else if c == 0 {
				if len(cancels) > 0 {
					log.Printf("Cancelling 1 sender of %d since queue is empty\n", len(cancels))
					cancels[0]()
					cancels = cancels[1:]
				}
			} else {
				log.Printf("CloudWatchSender heartbeat: %d events in queue, %d active senders\n", len(s.Queue), len(cancels))
			}
		case <-ctx.Done():
			log.Println("CloudWatchSender received shutdown signal")
			for _, cancel := range cancels {
				cancel()
			}
			s.flush(true)
			return
		}
	}
}

func makeInput[I PutMetricDataInput](ns string, dimensions []model.Dimension, evts []model.Event) (*I, error) {
	if len(evts) == 0 {
		return nil, fmt.Errorf("no events to send")
	}
	i := any(new(I))

	switch o := i.(type) {
	case *v1.PutMetricDataInput:
		o.MetricData = make([]*v1.MetricDatum, 0, len(evts))
		o.Namespace = Pointer(ns)
		for _, evt := range evts {
			var value = float64(0)
			if evt.Latency != nil {
				value = float64(*evt.Latency)
			} else if evt.AttemptLatency != nil {
				value = float64(*evt.AttemptLatency)
			} else if evt.RawValue != nil {
				value = float64(*evt.RawValue)
			}

			standardUnit := v1.StandardUnitMilliseconds
			if evt.StandardUnit != nil {
				standardUnit = *evt.StandardUnit
			}

			datum := &v1.MetricDatum{
				MetricName: evt.API,
				Value:      Pointer(value),
				Unit:       Pointer(standardUnit),
				Dimensions: make([]*v1.Dimension, 0, len(dimensions)),
				Timestamp:  (*time.Time)(evt.Timestamp),
			}

			for _, d := range evt.Dimensions {
				datum.Dimensions = append(datum.Dimensions, &v1.Dimension{
					Name:  d.Name,
					Value: d.Value,
				})
			}

			for _, d := range dimensions {
				datum.Dimensions = append(datum.Dimensions, &v1.Dimension{
					Name:  d.Name,
					Value: d.Value,
				})
			}

			if evt.Latency == nil && evt.AttemptLatency != nil {
				datum.MetricName = Pointer(fmt.Sprintf("%s.AttemptLatency", *evt.API))
			}

			o.MetricData = append(o.MetricData, datum)
		}
	case *v2.PutMetricDataInput:
		o.MetricData = make([]cwtypes.MetricDatum, 0, len(evts))
		o.Namespace = Pointer(ns)
		for _, evt := range evts {
			var latency = float64(0)
			if evt.Latency != nil {
				latency = float64(*evt.Latency)
			} else if evt.AttemptLatency != nil {
				latency = float64(*evt.AttemptLatency)
			} else if evt.RawValue != nil {
				latency = *evt.RawValue
			}

			standardUnit := cwtypes.StandardUnitMilliseconds
			if evt.StandardUnit != nil {
				standardUnit = cwtypes.StandardUnit(*evt.StandardUnit)
			}

			datum := cwtypes.MetricDatum{
				MetricName: evt.API,
				Value:      Pointer(latency),
				Unit:       standardUnit,
				Dimensions: make([]cwtypes.Dimension, 0, len(dimensions)),
				Timestamp:  (*time.Time)(evt.Timestamp),
			}

			for _, d := range evt.Dimensions {
				datum.Dimensions = append(datum.Dimensions, cwtypes.Dimension{
					Name:  d.Name,
					Value: d.Value,
				})
			}

			for _, d := range dimensions {
				datum.Dimensions = append(datum.Dimensions, cwtypes.Dimension{
					Name:  d.Name,
					Value: d.Value,
				})
			}

			if evt.Latency == nil && evt.AttemptLatency != nil {
				datum.MetricName = Pointer(fmt.Sprintf("%s.AttemptLatency", *evt.API))
			}

			o.MetricData = append(o.MetricData, datum)
		}
	default:
		return nil, fmt.Errorf("unsupported input type: %T", i)
	}
	return i.(*I), nil
}

func UnmarshalWithMetrics[
	T any,
	I PutMetricDataInput,
	O PutMetricDataOutput,
	AV map[string]*dynamodb.AttributeValue | map[string]types.AttributeValue,
](
	_ context.Context,
	item AV,
	sender *CloudWatchSender[I, O],
	unmarshaller func(AV, any) error,
) (*T, error) {
	var o T
	start := time.Now()
	err := unmarshaller(item, &o)
	end := time.Now()
	diff := end.Sub(start)

	metricName := "UnmarshalMap"
	if err != nil || len(item) == 0 {
		metricName = "UnmarshalMap.error"
	}

	event := model.Event{
		API:            Pointer(metricName),
		Latency:        Pointer(int(diff.Microseconds())),
		Type:           Pointer("AttributeValue"),
		Service:        Pointer("DynamoDB"),
		ClientID:       Pointer("v1"),
		Timestamp:      (*model.MetricTime)(Pointer(end)),
		Version:        Pointer(1),
		AttemptCount:   Pointer(1),
		AttemptLatency: nil,
		StandardUnit:   Pointer(cloudwatchlogs.StandardUnitMicroseconds),
	}
	sender.Send(event)

	return &o, err
}

func MarshalWithMetrics[
	T any,
	I PutMetricDataInput,
	O PutMetricDataOutput,
	AV map[string]*dynamodb.AttributeValue | map[string]types.AttributeValue,
](
	_ context.Context,
	item *T,
	sender *CloudWatchSender[I, O],
	marshaller func(any) (AV, error),
) (AV, error) {
	start := time.Now()
	userMap, err := marshaller(item)
	end := time.Now()
	diff := end.Sub(start)

	metricName := "MarshalMap"
	if err != nil || len(userMap) == 0 {
		metricName = "MarshalMap.error"
	}

	event := model.Event{
		API:            Pointer(metricName),
		Latency:        Pointer(int(diff.Microseconds())),
		Type:           Pointer("AttributeValue"),
		Service:        Pointer("DynamoDB"),
		ClientID:       Pointer("v1"),
		Timestamp:      (*model.MetricTime)(Pointer(end)),
		Version:        Pointer(1),
		AttemptCount:   Pointer(1),
		AttemptLatency: nil,
		StandardUnit:   Pointer(cloudwatchlogs.StandardUnitMicroseconds),
	}

	sender.Send(event)

	return userMap, err
}

func (s *CloudWatchSender[I, O]) Meter(scope string, opts ...metrics.MeterOption) metrics.Meter {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	if s.Meters == nil {
		s.Meters = make(map[string]metrics.Meter)
	}

	if m, exists := s.Meters[scope]; exists {
		return m
	}

	s.Meters[scope] = &Meter[I, O]{
		Scope:            scope,
		Opts:             opts,
		CloudWatchSender: s,
	}

	return s.Meters[scope]
}

type Meter[I PutMetricDataInput, O PutMetricDataOutput] struct {
	Scope            string
	Opts             []metrics.MeterOption
	CloudWatchSender *CloudWatchSender[I, O]

	Mutex sync.Mutex

	IntCounters       map[string]metrics.Int64Counter
	IntUpDownCounters map[string]metrics.Int64UpDownCounter
	IntGauges         map[string]metrics.Int64Gauge
	IntHistograms     map[string]metrics.Int64Histogram

	FloatCounter        map[string]metrics.Float64Counter
	FloatUpDownCounters map[string]metrics.Float64UpDownCounter
	FloatGauges         map[string]metrics.Float64Gauge
	FloatHistograms     map[string]metrics.Float64Histogram
}

func (m *Meter[I, O]) Int64Counter(name string, opts ...metrics.InstrumentOption) (metrics.Int64Counter, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.IntCounters == nil {
		m.IntCounters = make(map[string]metrics.Int64Counter)
	}

	if c, exists := m.IntCounters[name]; exists {
		return c, nil
	}

	m.IntCounters[name] = &Instrument[int64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.IntCounters[name], nil
}

func (m *Meter[I, O]) Int64UpDownCounter(name string, opts ...metrics.InstrumentOption) (metrics.Int64UpDownCounter, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.IntUpDownCounters == nil {
		m.IntUpDownCounters = make(map[string]metrics.Int64UpDownCounter)
	}

	if c, exists := m.IntUpDownCounters[name]; exists {
		return c, nil
	}

	m.IntUpDownCounters[name] = &Instrument[int64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.IntUpDownCounters[name], nil
}

func (m *Meter[I, O]) Int64Gauge(name string, opts ...metrics.InstrumentOption) (metrics.Int64Gauge, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.IntGauges == nil {
		m.IntGauges = make(map[string]metrics.Int64Gauge)
	}

	if c, exists := m.IntGauges[name]; exists {
		return c, nil
	}

	m.IntGauges[name] = &Instrument[int64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.IntGauges[name], nil
}

func (m *Meter[I, O]) Int64Histogram(name string, opts ...metrics.InstrumentOption) (metrics.Int64Histogram, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.IntHistograms == nil {
		m.IntHistograms = make(map[string]metrics.Int64Histogram)
	}

	if c, exists := m.IntHistograms[name]; exists {
		return c, nil
	}

	m.IntHistograms[name] = &Instrument[int64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.IntHistograms[name], nil
}

func (m *Meter[I, O]) Int64AsyncCounter(name string, callback metrics.Int64Callback, opts ...metrics.InstrumentOption) (metrics.AsyncInstrument, error) {
	panic("implement me: " + name)
}

func (m *Meter[I, O]) Int64AsyncUpDownCounter(name string, callback metrics.Int64Callback, opts ...metrics.InstrumentOption) (metrics.AsyncInstrument, error) {
	panic("implement me: " + name)
}

func (m *Meter[I, O]) Int64AsyncGauge(name string, callback metrics.Int64Callback, opts ...metrics.InstrumentOption) (metrics.AsyncInstrument, error) {
	panic("implement me: " + name)
}

func (m *Meter[I, O]) Float64Counter(name string, opts ...metrics.InstrumentOption) (metrics.Float64Counter, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.FloatCounter == nil {
		m.FloatCounter = make(map[string]metrics.Float64Counter)
	}

	if c, exists := m.FloatCounter[name]; exists {
		return c, nil
	}

	m.FloatCounter[name] = &Instrument[float64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.FloatCounter[name], nil
}

func (m *Meter[I, O]) Float64UpDownCounter(name string, opts ...metrics.InstrumentOption) (metrics.Float64UpDownCounter, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.FloatUpDownCounters == nil {
		m.FloatUpDownCounters = make(map[string]metrics.Float64UpDownCounter)
	}

	if c, exists := m.FloatUpDownCounters[name]; exists {
		return c, nil
	}

	m.FloatUpDownCounters[name] = &Instrument[float64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.FloatUpDownCounters[name], nil
}

func (m *Meter[I, O]) Float64Gauge(name string, opts ...metrics.InstrumentOption) (metrics.Float64Gauge, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.FloatGauges == nil {
		m.FloatGauges = make(map[string]metrics.Float64Gauge)
	}

	if c, exists := m.FloatGauges[name]; exists {
		return c, nil
	}

	m.FloatGauges[name] = &Instrument[float64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.FloatGauges[name], nil
}

func (m *Meter[I, O]) Float64Histogram(name string, opts ...metrics.InstrumentOption) (metrics.Float64Histogram, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.FloatHistograms == nil {
		m.FloatHistograms = make(map[string]metrics.Float64Histogram)
	}

	if c, exists := m.FloatHistograms[name]; exists {
		return c, nil
	}

	m.FloatHistograms[name] = &Instrument[float64, I, O]{
		Name:             name,
		CloudWatchSender: m.CloudWatchSender,
		Opts:             opts,
	}

	return m.FloatHistograms[name], nil
}

func (m *Meter[I, O]) Float64AsyncCounter(name string, callback metrics.Float64Callback, opts ...metrics.InstrumentOption) (metrics.AsyncInstrument, error) {
	panic("implement me: " + name)
}

func (m *Meter[I, O]) Float64AsyncUpDownCounter(name string, callback metrics.Float64Callback, opts ...metrics.InstrumentOption) (metrics.AsyncInstrument, error) {
	panic("implement me: " + name)
}

func (m *Meter[I, O]) Float64AsyncGauge(name string, callback metrics.Float64Callback, opts ...metrics.InstrumentOption) (metrics.AsyncInstrument, error) {
	panic("implement me: " + name)
}

type Instrument[T int64 | float64, I PutMetricDataInput, O PutMetricDataOutput] struct {
	Name             string
	CloudWatchSender *CloudWatchSender[I, O]
	Opts             []metrics.InstrumentOption
}

// gauge
func (i *Instrument[T, I, O]) Sample(_ context.Context, f T, opts ...metrics.RecordMetricOption) {
	i.CloudWatchSender.Send(model.Event{
		API:            Pointer(i.Name),
		Latency:        Pointer(int(f * 1_000_000)),
		Type:           Pointer("AttributeValue"),
		Service:        Pointer("DynamoDBv2"),
		ClientID:       Pointer("v2"),
		Timestamp:      (*model.MetricTime)(Pointer(time.Now())),
		Version:        Pointer(2),
		AttemptCount:   Pointer(1),
		AttemptLatency: nil,
		StandardUnit:   Pointer(string(cwtypes.StandardUnitMicroseconds)),
		Dimensions:     optsToDimensions(opts),
	})
}

// counter
func (i *Instrument[T, I, O]) Add(_ context.Context, f T, opts ...metrics.RecordMetricOption) {
	i.CloudWatchSender.Send(model.Event{
		API:          Pointer(i.Name),
		Latency:      Pointer(int(f * 1_000_000)),
		Type:         Pointer("AttributeValue"),
		Service:      Pointer("DynamoDBv2"),
		ClientID:     Pointer("v2"),
		Timestamp:    (*model.MetricTime)(Pointer(time.Now())),
		Version:      Pointer(2),
		AttemptCount: Pointer(1),
		StandardUnit: Pointer(string(cwtypes.StandardUnitMicroseconds)),
		Dimensions:   optsToDimensions(opts),
	})
}

// histogram
func (i *Instrument[T, I, O]) Record(_ context.Context, f T, opts ...metrics.RecordMetricOption) {
	i.CloudWatchSender.Send(model.Event{
		API:          Pointer(i.Name),
		Latency:      Pointer(int(f * 1_000_000)),
		Type:         Pointer("AttributeValue"),
		Service:      Pointer("DynamoDBv2"),
		ClientID:     Pointer("v2"),
		Timestamp:    (*model.MetricTime)(Pointer(time.Now())),
		Version:      Pointer(2),
		AttemptCount: Pointer(1),
		StandardUnit: Pointer(string(cwtypes.StandardUnitMicroseconds)),
		Dimensions:   optsToDimensions(opts),
	})
}

func optsToDimensions(opts []metrics.RecordMetricOption) []model.Dimension {
	out := make([]model.Dimension, 0, len(opts))
	o := metrics.RecordMetricOptions{}
	for _, opt := range opts {
		opt(&o)
	}

	for k, v := range o.Properties.Values() {
		ks, kOk := k.(string)
		vs, sOk := v.(string)
		if !sOk || !kOk {
			continue
		}
		if !inArray(allowedOpts, ks) {
			continue
		}
		out = append(out, model.Dimension{
			Name:  Pointer(ks),
			Value: Pointer(vs),
		})
	}

	return out
}

func inArray[S ~[]E, E comparable](s S, v E) bool {
	for _, a := range s {
		if a == v {
			return true
		}
	}
	return false
}

var allowedOpts = []string{
	"rpc.method",
}
