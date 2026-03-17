package tdx

import (
	"strings"
)

var (
	SHHosts = []string{
		"124.71.187.122",
		"122.51.120.217",
		"111.229.247.189",
		"124.70.176.52",
		"123.60.186.45",
		"122.51.232.182",
		"118.25.98.114",
		"124.70.199.56",
		"121.36.225.169",
		"123.60.70.228",
		"123.60.73.44",
		"124.70.133.119",
		"124.71.187.72",
		"123.60.84.66",
	}

	BJHosts = []string{
		"121.36.54.217",
		"121.36.81.195",
		"123.249.15.60",
		"124.70.75.113",
		"120.46.186.223",
		"124.70.22.210",
		"139.9.133.247",
	}

	GZHosts = []string{
		"124.71.85.110",
		"139.9.51.18",
		"139.159.239.163",
		"124.71.9.153",
		"116.205.163.254",
		"116.205.171.132",
		"116.205.183.150",
		"111.230.186.52",
		"110.41.4.4",
		"110.41.2.72",
		"110.41.154.219",
		"110.41.147.114",
	}

	WHHosts = []string{
		"119.97.185.59",
	}
)

func GetAllHosts() []string {
	hosts := make([]string, 0)
	hosts = append(hosts, SHHosts...)
	hosts = append(hosts, BJHosts...)
	hosts = append(hosts, GZHosts...)
	hosts = append(hosts, WHHosts...)
	return hosts
}

func FormatHost(host string) string {
	if !strings.Contains(host, ":") {
		return host + ":7709"
	}
	return host
}
