package mpawsalb

import (
	"flag"

	mp "github.com/mackerelio/go-mackerel-plugin-helper"
	"github.com/mackerelio/golib/logging"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/ec2metadata"
	"log"
	"github.com/aws/aws-sdk-go/service/cloudwatch"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"time"
	"errors"
	"regexp"
	"strings"
)

var logger = logging.GetLogger("metrics.plugin.aws-alb")


var graphdef = map[string]mp.Graphs{
	// ELB metrics
	// ProcessedBytes(Sum)            : alb.bytes.processed
	"alb.bytes.processed": {
		Label: "Processed Bytes",
		Unit:  "bytes",
		Metrics: []mp.Metrics{
			{Name: "ProcessedBytes", Label: "Processed", Stacked: false, },
		},
	},
	// NewConnectionCount(Sum)        : alb.connection_count.new
	// RejectedConnectionCount(Sum)   : alb.connection_count.rejected
	"alb.connection_count": {
		Label: "Connection Count",
		Unit:  "integer",
		Metrics: []mp.Metrics{
			{Name: "NewConnectionCount", Label: "New", Stacked: false},
			{Name: "RejectedConnectionCount", Label: "Rejected", Stacked: false},
		},
	},
	// ActiveConnectionCount(Sum)     : alb.concurrent_connection_count.active
	"alb.concurrent_connection_count": {
		Label: "Concurrent Connection Count",
		Unit:  "integer",
		Metrics: []mp.Metrics{
			{Name: "ActiveConnectionCount", Label: "Active", Stacked: false},
		},
	},
	// TargetConnectionErrorCount(Sum): alb.connection_error_count.target
	"alb.connection_error_count": {
		Label: "Target Connection Error Count",
		Unit:  "integer",
		Metrics: []mp.Metrics{
			{Name: "TargetConnectionErrorCount", Label: "Count", Stacked: false},
		},
	},
	// RequestCount(Sum)              : alb.request.count
	"alb.request": {
		Label: "Concurrent Connection Count",
		Unit:  "integer",
		Metrics: []mp.Metrics{
			{Name: "RequestCount", Label: "Request Count", Stacked: false},
		},
	},
	// TargetResponseTime(Avg) : alb.response.time'
	"alb.response": {
		Label: "Target Connection Error Count",
		Unit:  "float",
		Metrics: []mp.Metrics{
			{Name: "TargetResponseTime", Label: "Time", Stacked: false},
		},
	},
	// HTTPCode_ELB_5XX_Count(Sum)    : alb.httpcode_count.alb_5xx
	// HTTPCode_ELB_4XX_Count(Sum)    : alb.httpcode_count.alb_4xx
	// HTTPCode_Target_2XX_Count      : alb.httpcode_count.target_2xx
	// HTTPCode_Target_3XX_Count      : alb.httpcode_count.target_3xx
	// HTTPCode_Target_4XX_Count      : alb.httpcode_count.target_4xx
	// HTTPCode_Target_5XX_Count      : alb.httpcode_count.target_5xx
	"alb.httpcode_count": {
		Label: "HTTP Code Count",
		Unit:  "integer",
		Metrics: []mp.Metrics{
			{Name: "HTTPCode_ELB_4XX_Count", Label: "ALB 4XX", Stacked: false},
			{Name: "HTTPCode_ELB_5XX_Count", Label: "ALB 5XX", Stacked: false},
			{Name: "HTTPCode_Target_2XX_Count", Label: "Target 2XX", Stacked: false},
			{Name: "HTTPCode_Target_3XX_Count", Label: "Target 3XX", Stacked: false},
			{Name: "HTTPCode_Target_4XX_Count", Label: "Target 4XX", Stacked: false},
			{Name: "HTTPCode_Target_5XX_Count", Label: "Target 5XX", Stacked: false},
		},
	},
	// TargetGroup metrics

	// HealthyHostCount(Avg)          : alb.host_count.#.healthy
	// UnHealthyHostCount(Avg)        : alb.host_count.#.unhealthy
	"alb.host_count.#": {
		Label: "Host Count",
		Unit:  "float",
		Metrics: []mp.Metrics{
			{Name: "HealthyHostCount", Label: "Healthy", Stacked: false},
			{Name: "UnHealthyHostCount", Label: "UnHealthy", Stacked: false},
		},
	},
	// HTTPCode_Target_2XX_Count(Sum) : alb.httpcode_count_per_group.#.target_2xx
	// HTTPCode_Target_3XX_Count(Sum) : alb.httpcode_count_per_group.#.target_2xx
	// HTTPCode_Target_4XX_Count(Sum) : alb.httpcode_count_per_group.#.target_2xx
	// HTTPCode_Target_5XX_Count(Sum) : alb.httpcode_count_per_group.#.target_2xx
	"alb.httpcode_count_per_group.#": {
		Label: "HTTP Code Count",
		Unit:  "integer",
		Metrics: []mp.Metrics{
			{Name: "HTTPCode_Target_2XX_Count", Label: "Target 2XX", Stacked: false},
			{Name: "HTTPCode_Target_3XX_Count", Label: "Target 3XX", Stacked: false},
			{Name: "HTTPCode_Target_4XX_Count", Label: "Target 4XX", Stacked: false},
			{Name: "HTTPCode_Target_5XX_Count", Label: "Target 5XX", Stacked: false},
		},
	},
	// RequestCountPerTarget(Sum)     : alb.request_per_group.#.count
	"alb.request_per_group.#": {
		Label: "Concurrent Connection Count",
		Unit:  "integer",
		Metrics: []mp.Metrics{
			{Name: "RequestCountPerTarget", Label: "Request Count", Stacked: false},
		},
	},
	// TargetResponseTime(Avg)        : alb.response_per_group.#.time
	"alb.response_per_group.#": {
		Label: "Target Connection Error Count",
		Unit:  "float",
		Metrics: []mp.Metrics{
			{Name: "TargetResponseTime", Label: "Time", Stacked: false},
		},
	},
}

type statType int

const (
	stAve statType = iota
	stSum
)

func (s statType) String() string {
	switch s {
	case stAve:
		return "Average"
	case stSum:
		return "Sum"
	}
	return ""
}

// ALBPlugin alb plugin for mackerel
type ALBPlugin struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	AZs             []*string
	CloudWatch      *cloudwatch.CloudWatch
	Lbname          string
	Tgnames			[]string
	FetchDuration	int
}

func (p *ALBPlugin) prepare() error {
	sess := session.Must(session.NewSession())

	config := aws.NewConfig()
	if p.AccessKeyID != "" && p.SecretAccessKey != "" {
		config = config.WithCredentials(credentials.NewStaticCredentials(p.AccessKeyID, p.SecretAccessKey, ""))
	}
	if p.Region != "" {
		config = config.WithRegion(p.Region)
	}

	p.CloudWatch = cloudwatch.New(sess, config)

	r := regexp.MustCompile("app/(.+?)/.+")
	groups := r.FindStringSubmatch(p.Lbname)
	elbSimpleName := p.Lbname
	if len(groups) > 1 {
		elbSimpleName = groups[1]
	}

	elbv2Client := elbv2.New(sess, config)
	lbs, err := elbv2Client.DescribeLoadBalancers(&elbv2.DescribeLoadBalancersInput{
		Names: []*string{aws.String(elbSimpleName)},
	})
	if err == nil && len(lbs.LoadBalancers) == 1{
		tgs, err := elbv2Client.DescribeTargetGroups(&elbv2.DescribeTargetGroupsInput{
			LoadBalancerArn: aws.String(*lbs.LoadBalancers[0].LoadBalancerArn),
		})
		tr := regexp.MustCompile("arn:aws:elasticloadbalancing:.+:(.+)")
		if err == nil && len(tgs.TargetGroups) > 0 {
			tgnames := make([]string, 1)
			for _, tg := range tgs.TargetGroups {
				tgroups := tr.FindStringSubmatch(*tg.TargetGroupArn)
				tgnames = append(tgnames, tgroups[1])
			}
			p.Tgnames = tgnames
		}
	}

	return nil
}

func (p ALBPlugin) getLastPoint(dimensions []*cloudwatch.Dimension, metricName string, sTyp statType) (float64, error) {
	time.Local = time.UTC
	now := time.Now().Truncate(time.Duration(time.Minute))

	response, err := p.CloudWatch.GetMetricStatistics(&cloudwatch.GetMetricStatisticsInput{
		Dimensions: dimensions,
		StartTime:  aws.Time(now.Add(time.Duration(p.FetchDuration) * time.Second * -1)), // 5 min (to fetch at least 1 data-point)
		EndTime:    aws.Time(now),
		MetricName: aws.String(metricName),
		Period:     aws.Int64(60),
		Statistics: []*string{aws.String(sTyp.String())},
		Namespace:  aws.String("AWS/ApplicationELB"),
	})
	if err != nil {
		return 0, err
	}
	datapoints := response.Datapoints
	if len(datapoints) == 0 {
		return 0, errors.New("fetched no datapoints")
	}

	latest := new(time.Time)
	var latestVal float64
	for _, dp := range datapoints {
		if dp.Timestamp.Before(*latest) {
			continue
		}

		latest = dp.Timestamp
		switch sTyp {
		case stAve:
			latestVal = *dp.Average
		case stSum:
			latestVal = *dp.Sum
		}
	}

	return latestVal, nil
}

// FetchMetrics fetch elb metrics
func (p ALBPlugin) FetchMetrics() (map[string]interface{}, error) {
	stat := make(map[string]interface{})
	if p.Lbname != "" {
		ld := []*cloudwatch.Dimension{
			{
				Name:  aws.String("LoadBalancer"),
				Value: aws.String(p.Lbname),
			},
		}
		for _, met := range []string{
			"ProcessedBytes",
			"NewConnectionCount", "RejectedConnectionCount", "ActiveConnectionCount", "TargetConnectionErrorCount",
			"RequestCount",
			"HTTPCode_ELB_5XX_Count", "HTTPCode_ELB_4XX_Count",
			"HTTPCode_Target_2XX_Count", "HTTPCode_Target_3XX_Count", "HTTPCode_Target_4XX_Count", "HTTPCode_Target_5XX_Count",
		} {
			v, err := p.getLastPoint(ld, met, stSum)
			if err == nil {
				stat[met] = v
			}
		}
		for _, met := range []string{"TargetResponseTime"} {
			v, err := p.getLastPoint(ld, met, stAve)
			if err == nil {
				stat[met] = v
			}
		}

		tgr := regexp.MustCompile("targetgroup/(.+?)/.+")

		for _, tgname := range p.Tgnames {
			group := tgr.FindStringSubmatch(tgname)
			tgMetricName := ""
			if len(group) >= 2 {
				tgMetricName = group[1]
			} else {
				tgMetricName = strings.Replace(tgname, "/", "_", 0)
			}
			td := []*cloudwatch.Dimension{
				{
					Name:  aws.String("LoadBalancer"),
					Value: aws.String(p.Lbname),
				},
				{
					Name:  aws.String("TargetGroup"),
					Value: aws.String(tgname),
				},
			}
			for _, met := range []string{
				"HTTPCode_Target_2XX_Count", "HTTPCode_Target_3XX_Count", "HTTPCode_Target_4XX_Count", "HTTPCode_Target_5XX_Count",
			} {
				v, err := p.getLastPoint(td, met, stSum)
				if err == nil {
					stat["alb.httpcode_count_per_group." + tgMetricName + "." + met] = v
				}
			}
			for _, met := range []string{
				"RequestCountPerTarget",
			} {
				v, err := p.getLastPoint(td, met, stSum)
				if err == nil {
					stat["alb.request_per_group." + tgMetricName + "." + met] = v
				}
			}
			for _, met := range []string{"HealthyHostCount", "UnHealthyHostCount",} {
				v, err := p.getLastPoint(td, met, stAve)
				if err == nil {
					stat["alb.host_count." + tgMetricName + "." + met] = v
				}
			}
			for _, met := range []string{"TargetResponseTime",} {
				v, err := p.getLastPoint(td, met, stAve)
				if err == nil {
					stat["alb.response_per_group." + tgMetricName + "." + met] = v
				}
			}
		}
	}
	return stat, nil
}

// GraphDefinition for Mackerel
func (p ALBPlugin) GraphDefinition() map[string]mp.Graphs {
	return graphdef
}

// Do the plugin
func Do() {
	optRegion := flag.String("region", "", "AWS Region")
	optLbname := flag.String("lbname", "", "ELB Name")
	optTgname := flag.String("tgname", "", "TargetGroup Name")
	optFetchDuration := flag.Int("fetch", 300, "Fetch Duration seconds")
	optAccessKeyID := flag.String("access-key-id", "", "AWS Access Key ID")
	optSecretAccessKey := flag.String("secret-access-key", "", "AWS Secret Access Key")
	optTempfile := flag.String("tempfile", "", "Temp file name")
	flag.Parse()

	var alb ALBPlugin
	sess := session.Must(session.NewSession())

	if *optRegion == "" {
		meta := ec2metadata.New(sess)
		if meta.Available() {
			alb.Region, _ = meta.Region()
		}
	} else {
		alb.Region = *optRegion
	}
	alb.AccessKeyID = *optAccessKeyID
	alb.SecretAccessKey = *optSecretAccessKey
	alb.Lbname = *optLbname
	splitTbNames := strings.Split(*optTgname, ",")
	alb.Tgnames = splitTbNames
	alb.FetchDuration = *optFetchDuration

	err := alb.prepare()
	if err != nil {
		log.Fatalln(err)
	}

	helper := mp.NewMackerelPlugin(alb)
	helper.Tempfile = *optTempfile

	helper.Run()
}
