mackerel-plugin-aws-alb
===========================

AWS ALB custom metrics plugin for mackerel.io agent.

## Synopsis

```shell
mackerel-plugin-aws-alb [-lbname=<aws-load-blancer-name>] [-tgname=<aws-load-blancer-name>] [-region=<aws-region>] [-access-key-id=<id>] [-secret-access-key=<key>] [-tempfile=<tempfile>]
```
* if you run on an ec2-instance, you probably don't have to specify `-region`
* if you run on an ec2-instance and the instance is associated with an appropriate IAM Role, you probably don't have to specify `-access-key-id` & `-secret-access-key`
* if you set lbname only, then fetch all target group metrics on specified ALB.

## AWS IAM Policy
the credential provided manually or fetched automatically by IAM Role should have the policy that includes actions, 
- `cloudwatch:GetMetricStatistics` 
- `cloudwatch:ListMetrics`
- `elasticloadbalancing:DescribeLoadBalancers`
- `elasticloadbalancing:DescribeTargetGroups`

## Example of mackerel-agent.conf

```
[plugin.metrics.aws-elb]
command = "/path/to/mackerel-plugin-aws-alb"
```

## Standalone usage

```
echo -e "$(MACKEREL_AGENT_PLUGIN_META=1 go run main.go -region=ap-northeast-1 -lbname=app/lb-name/1234567890)\n$(go run main.go -region=ap-northeast-1 -lbname=app/lb-name/1234567890 -tempfile=/tmp/alb-metrics )" |  /usr/local/bin/mkr throw -s alb-service-metrics
```