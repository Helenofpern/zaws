package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/AlekSi/zabbix-sender"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/aws/aws-sdk-go/service/rds"
	//"github.com/aws/aws-sdk-go/service/sqs"
	"net"
	"os"
	"strconv"
	"time"
)

type Zaws struct {
	Region      string
	AccessKeyId string
	SecretKeyId string
	TargetId    string
	MetricName  string
	ZabbixHost  string
	ZabbixPort  string
	AwsSession  *session.Session
}

func NewZaws() *Zaws {
	zaws := new(Zaws)
	zaws.SetOption()
	zaws.AwsSession = session.New(&aws.Config{
		Region:      aws.String(zaws.Region),
		Credentials: credentials.NewStaticCredentials(zaws.AccessKeyId, zaws.SecretKeyId, ""),
	})
	return zaws
}

func (z *Zaws) SetOption() {
	flag.StringVar(&z.Region, "region", "ap-northeast-1", "Set AWS region")
	flag.StringVar(&z.Region, "r", "ap-northeast-1", "Set AWS region")
	flag.StringVar(&z.AccessKeyId, "key", os.Getenv("AWS_ACCESS_KEY_ID"), "Set AWS API Access key id")
	flag.StringVar(&z.AccessKeyId, "k", os.Getenv("AWS_ACCESS_KEY_ID"), "Set AWS API Access key id")
	flag.StringVar(&z.SecretKeyId, "secret", os.Getenv("AWS_SECRET_ACCESS_KEY"), "Set AWS API Secret key id")
	flag.StringVar(&z.SecretKeyId, "s", os.Getenv("AWS_SECRET_ACCESS_KEY"), "Set AWS API Secret key id")
	flag.StringVar(&z.TargetId, "id", "", "Set target object id")
	flag.StringVar(&z.TargetId, "i", "", "Set target object id")
	flag.StringVar(&z.MetricName, "metric", "", "Set metric name")
	flag.StringVar(&z.MetricName, "m", "", "Set metric name")
	flag.StringVar(&z.ZabbixHost, "host", "localhost", "Set zabbix host name")
	flag.StringVar(&z.ZabbixHost, "h", "localhost", "Set zabbix host name")
	flag.StringVar(&z.ZabbixPort, "port", "10051", "Set zabbix host port")
	flag.StringVar(&z.ZabbixPort, "p", "10051", "Set zabbix host port")
	flag.Parse()
	if z.AccessKeyId == "" || z.SecretKeyId == "" {
		fmt.Println("[ERROR]: Please set key information")
		usage()
	}
}

// Declare Struct
type LldJson struct {
	Data []Data `json:"data"`
}

type Data struct {
	MetricName          string `json:"{#METRIC.NAME},omitempty"`
	MetricUnit          string `json:"{#METRIC.UNIT},omitempty"`
	MetricNamespace     string `json:"{#METRIC.NAMESPACE},omitempty"`
	InstanceName        string `json:"{#INSTANCE.NAME},omitempty"`
	InstanceType        string `json:"{#INSTANCE.TYPE},omitempty"`
	InstanceId          string `json:"{#INSTANCE.ID},omitempty"`
	InstancePrivateAddr string `json:"{#INSTANCE.PRIVATE.ADDR},omitempty"`
	ElbName             string `json:"{#ELB.NAME},omitempty"`
	ElbDnsName          string `json:"{#ELB.DNS.NAME},omitempty"`
	RdsName             string `json:"{#RDS.NAME},omitempty"`
	RdsDnsName          string `json:"{#RDS.DNS.NAME},omitempty"`
}

// Common util

func usage() {
	fmt.Println("Usage: zaws service method [target] [-region|-r] [-key|-k] [-secret|-s] [-id|-i] [-metric|-m] [-host|h] [-port|p]")
	os.Exit(1)
}

func convert_to_lldjson_string(data []Data) string {
	lld_json := LldJson{data}
	convert_json, _ := json.Marshal(lld_json)
	return string(convert_json)
}

// Access AWS API
func get_metric_list(sess *session.Session, identity_name, target_id string) []*cloudwatch.Metric {
	svc := cloudwatch.New(sess)
	params := &cloudwatch.ListMetricsInput{
		Dimensions: []*cloudwatch.DimensionFilter{
			{
				Name:  aws.String(identity_name),
				Value: aws.String(target_id),
			},
		},
	}
	resp, err := svc.ListMetrics(params)
	if err != nil {
		fmt.Printf("[ERROR] Fail ListMetrics API call: %s \n", err.Error())
		return nil
	}
	return resp.Metrics
}

func map_metric_name_to_statistics (metric_name string) string {

	switch metric_name {
	case "ApproximateAgeOfOldestMessage":
		return "Maximum"

	case "NumberOfMessagesDeleted" :
		fallthrough
	case "NumberOfEmptyReceives" :
		fallthrough
	case "ApproximateNumberOfMessagesVisible":
		fallthrough
	case "ApproximateNumberOfMessagesNotVisible":
		fallthrough
	case "NumberOfMessagesSent":
		fallthrough
	case "ApproximateNumberOfMessagesDelayed":
		fallthrough
	case "NumberOfMessagesReceived":
		return "Sum"

	default:
		return "Average"


	}
}

func get_datapont_value(metric_name string, datapoint *cloudwatch.Datapoint) float64 {
	switch metric_name {
	case "ApproximateAgeOfOldestMessage":
		return *datapoint.Maximum

	case "NumberOfMessagesDeleted":
		fallthrough
	case "NumberOfEmptyReceives":
		fallthrough
	case "ApproximateNumberOfMessagesVisible":
		fallthrough
	case "ApproximateNumberOfMessagesNotVisible":
		fallthrough
	case "NumberOfMessagesSent":
		fallthrough
	case "ApproximateNumberOfMessagesDelayed":
		fallthrough
	case "NumberOfMessagesReceived":
		return *datapoint.Sum

	default:
		return *datapoint.Average
	}
}

func get_metric_stats(sess *session.Session, identity_name, target_id, metric_name, metric_namespace string) []*cloudwatch.Datapoint {

	svc := cloudwatch.New(sess)
	t := time.Now()
	input := &cloudwatch.GetMetricStatisticsInput{
		Namespace:  aws.String(metric_namespace),
		Statistics: []*string{aws.String(map_metric_name_to_statistics(metric_name))},
		EndTime:    aws.Time(t),
		Period:     aws.Int64(300),
		StartTime:  aws.Time(t.Add(time.Duration(-10) * time.Minute)),
		MetricName: aws.String(metric_name),
		Dimensions: []*cloudwatch.Dimension{
			{
				Name:  aws.String(identity_name),
				Value: aws.String(target_id),
			},
		},
	}
	value, err := svc.GetMetricStatistics(input)
	if err != nil {
		fmt.Printf("[ERROR] Fail GetMetricStatistics API call: %s \n", err.Error())
		return nil
	}
	return value.Datapoints
}

func get_ec2_list(sess *session.Session) []*ec2.Instance {
	var instances []*ec2.Instance
	svc := ec2.New(sess)
	resp, err := svc.DescribeInstances(nil)

	if err != nil {
		fmt.Printf("[ERROR] Fail DescribeInstances API call: %s \n", err.Error())
		os.Exit(1)
	}
	for _, reservation := range resp.Reservations {
		instances = append(instances, reservation.Instances...)
	}
	return instances
}

func get_rds_list(sess *session.Session) []*rds.DBInstance {

	svc := rds.New(sess)

	resp, err := svc.DescribeDBInstances(nil)

	if err != nil {
		fmt.Printf("[ERROR] Fail DescribeDBInstances API call: %s \n", err.Error())
		os.Exit(1)
	}

	return resp.DBInstances
}

func get_elb_list(sess *session.Session) []*elb.LoadBalancerDescription {
	svc := elb.New(sess)
	params := &elb.DescribeLoadBalancersInput{
		LoadBalancerNames: []*string{},

	}
	resp, err := svc.DescribeLoadBalancers(params)

	if err != nil {
		fmt.Printf("[ERROR] Fail DescribeLoadBalancers API call: %s \n", err.Error())
		return nil
	}
	return resp.LoadBalancerDescriptions
}

// zaws method
func (z *Zaws) ShowEc2List() {
	list := make([]Data, 0)
	instances := get_ec2_list(z.AwsSession)
	for _, instance := range instances {
		data := Data{InstanceType: *instance.InstanceType, InstanceId: *instance.InstanceId}
		if instance.PrivateIpAddress != nil {
			data.InstancePrivateAddr = *instance.PrivateIpAddress
		}
		for _, tag := range instance.Tags {
			if *tag.Key == "Name" {
				data.InstanceName = *tag.Value
			}
		}
		if data.InstanceName == "" {
			data.InstanceName = *instance.InstanceId
		}
		list = append(list, data)
	}
	fmt.Printf(convert_to_lldjson_string(list))
}

func (z *Zaws) ShowElbList() {
	list := make([]Data, 0)
	elbs := get_elb_list(z.AwsSession)
	for _, elb_data := range elbs {
		data := Data{ElbName: *elb_data.LoadBalancerName, ElbDnsName: *elb_data.DNSName}
		list = append(list, data)
	}
	fmt.Printf(convert_to_lldjson_string(list))
}

func (z *Zaws) ShowRdsList() {
	list := make([]Data, 0)
	rds_s := get_rds_list(z.AwsSession)
	for _, rds_v := range rds_s {
		data := Data{RdsName: *rds_v.DBInstanceIdentifier, RdsDnsName: *rds_v.Endpoint.Address}
		list = append(list, data)
	}
	fmt.Printf(convert_to_lldjson_string(list))
}

func map_unit_name(unit string) string {

	switch unit {
	case "Count":
		return " "
	case "Seconds":
		return "s"

	case "":
		return " "

	case "Percent" :
		return "%"

	case "Count/Second":
		return "/s"

	case "Milliseconds":
		return "ms"

	case "Bytes":
		return "B"

	case "Bytes/Second":
		return "B/s"

	default:
		return unit
	}
}

func (z *Zaws) ShowEC2CloudwatchMetricsList() {
	list := make([]Data, 0)
	metrics := get_metric_list(z.AwsSession, "InstanceId", z.TargetId)
	for _, metric := range metrics {
		datapoints := get_metric_stats(z.AwsSession, "InstanceId", z.TargetId, *metric.MetricName, *metric.Namespace)
		data := Data{MetricName: *metric.MetricName, MetricNamespace: *metric.Namespace}
		if len(datapoints) > 0 {
			data.MetricUnit = map_unit_name(*datapoints[0].Unit)
		}
		list = append(list, data)
	}

	fmt.Println(convert_to_lldjson_string(list))
}

func (z *Zaws) ShowRDSCloudwatchMetricsList() {
	list := make([]Data, 0)
	metrics := get_metric_list(z.AwsSession, "DBInstanceIdentifier", z.TargetId)
	for _, metric := range metrics {
		datapoints := get_metric_stats(z.AwsSession, "DBInstanceIdentifier", z.TargetId, *metric.MetricName, *metric.Namespace)
		data := Data{MetricName: *metric.MetricName, MetricNamespace: *metric.Namespace}
		if len(datapoints) > 0 {
			data.MetricUnit = map_unit_name(*datapoints[0].Unit)
		}
		list = append(list, data)
	}

	fmt.Println(convert_to_lldjson_string(list))
}

func (z *Zaws) ShowELBCloudwatchMetricsList() {
	list := make([]Data, 0)
	metrics := get_metric_list(z.AwsSession, "LoadBalancerName", z.TargetId)
	for _, metric := range metrics {
		datapoints := get_metric_stats(z.AwsSession, "LoadBalancerName", z.TargetId, *metric.MetricName, *metric.Namespace)
		metric_name := *metric.MetricName
		for _, dimension := range metric.Dimensions {
			if *dimension.Name == "AvailabilityZone" {
				metric_name = *metric.MetricName + "." + *dimension.Value
				break
			}
		}
		data := Data{MetricName: metric_name, MetricNamespace: *metric.Namespace}
		if len(datapoints) > 0 {
			data.MetricUnit = map_unit_name(*datapoints[0].Unit)
		}
		list = append(list, data)
	}

	fmt.Println(convert_to_lldjson_string(list))
}

func (z *Zaws) ShowSQSCloudwatchMetricsList() {
	list := make([]Data, 0)
	metrics := get_metric_list(z.AwsSession, "QueueName", z.TargetId)
	for _, metric := range metrics {
		datapoints := get_metric_stats(z.AwsSession, "QueueName", z.TargetId, *metric.MetricName, *metric.Namespace)
		metric_name := *metric.MetricName
		for _, dimension := range metric.Dimensions {
			if *dimension.Name == "AvailabilityZone" {
				metric_name = *metric.MetricName + "." + *dimension.Value
				break
			}
		}
		data := Data{MetricName: metric_name, MetricNamespace: *metric.Namespace}
		if len(datapoints) > 0 {
			data.MetricUnit = map_unit_name(*datapoints[0].Unit)
		}
		list = append(list, data)
	}

	fmt.Println(convert_to_lldjson_string(list))
}

func (z *Zaws) SendEc2MetricStats() {
	z.SendMetricStats("InstanceId")
}
func (z *Zaws) SendRdsMetricStats() {
	z.SendMetricStats("DBInstanceIdentifier")
}
func (z *Zaws) SendElbMetricStats() {
	z.SendMetricStats("LoadBalancerName")
}
func (z *Zaws) SendSQSMetricStats() {
	z.SendMetricStats("QueueName")
}

func (z *Zaws) SendMetricStats(identity_name string) {
	var send_data []zabbix_sender.DataItem

	metrics := get_metric_list(z.AwsSession, identity_name, z.TargetId)
	for _, metric := range metrics {
		datapoints := get_metric_stats(z.AwsSession, identity_name, z.TargetId, *metric.MetricName, *metric.Namespace)
		metric_name := *metric.MetricName
		for _, dimension := range metric.Dimensions {
			if *dimension.Name == "AvailabilityZone" {
				metric_name = *metric.MetricName + "." + *dimension.Value
				break
			}
		}

		if len(datapoints) > 0 {
			data_time := *datapoints[0].Timestamp
			send_data = append(send_data, zabbix_sender.DataItem{Hostname: z.TargetId, Key: "cloudwatch.metric[" + metric_name + "]", Value: strconv.FormatFloat( get_datapont_value(metric_name, datapoints[0]), 'f', 4, 64), Timestamp: data_time.Unix()})
		}
	}
	addr, _ := net.ResolveTCPAddr("tcp", z.ZabbixHost+":"+z.ZabbixPort)
	res, err := zabbix_sender.Send(addr, send_data)
	if err != nil {
		fmt.Printf("[ERROR]: zabbix sender error!: %s", err)
		os.Exit(1)
	}
	fmt.Printf("[INFO]: Successful sending data to Zabbix: resp: %s", res)
}

func main() {
	if len(os.Args) < 3 {
		usage()
	}
	switch os.Args[1] {
	case "ec2":
		switch os.Args[2] {
		case "list":
			os.Args = os.Args[2:]
			zaws := NewZaws()
			zaws.ShowEc2List()
		default:
			usage()
		}
	case "elb":
		switch os.Args[2] {
		case "list":
			os.Args = os.Args[2:]
			zaws := NewZaws()
			zaws.ShowElbList()
		default:
			usage()
		}
	case "rds":
		switch os.Args[2] {
		case "list":
			os.Args = os.Args[2:]
			zaws := NewZaws()
			zaws.ShowRdsList()
		default:
			usage()
		}
	case "cloudwatch":
		switch os.Args[2] {
		case "list":
			if len(os.Args) < 4 {
				usage()
			}
			switch os.Args[3] {
			case "ec2":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.ShowEC2CloudwatchMetricsList()
			case "rds":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.ShowRDSCloudwatchMetricsList()
			case "elb":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.ShowELBCloudwatchMetricsList()
			case "sqs":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.ShowSQSCloudwatchMetricsList()
			default:
				usage()
			}
		case "stats":
			if len(os.Args) < 4 {
				usage()
			}
			switch os.Args[3] {
			case "ec2":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.SendEc2MetricStats()
			case "rds":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.SendRdsMetricStats()
			case "elb":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.SendElbMetricStats()
			case "sqs":
				os.Args = os.Args[3:]
				zaws := NewZaws()
				zaws.SendSQSMetricStats()
			default:
				usage()
			}
		default:
			usage()
		}

	default:
		usage()
	}
	os.Exit(0)
}
