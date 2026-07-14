package prometheus

import (
	"fmt"
)

const (
	QueryCPU    = `100 - (avg by(instance)(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`
	QueryMemory = `(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100`
	QueryDisk   = `(1 - node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"}) * 100`
	QueryLoad1  = `node_load1`
	QueryLoad5  = `node_load5`
	QueryLoad15 = `node_load15`

	QueryNetworkRx = `rate(node_network_receive_bytes_total{device!~"lo|veth.*|docker.*|br-.*"}[5m])`
	QueryNetworkTx = `rate(node_network_transmit_bytes_total{device!~"lo|veth.*|docker.*|br-.*"}[5m])`

	QueryDiskReadBytes  = `rate(node_disk_read_bytes_total[5m])`
	QueryDiskWriteBytes = `rate(node_disk_written_bytes_total[5m])`

	QueryUptime = `node_time_seconds - node_boot_time_seconds`
)

var thresholds = map[string]float64{
	"cpu":    80.0,
	"memory": 85.0,
	"disk":   90.0,
	"load":   4.0,
}

func (c *Client) CPU(server string) (*MetricsResponse, error) {
	result, err := c.Query(QueryCPU, server)
	if err != nil {
		return nil, err
	}
	resp := c.parseResults(result, thresholds["cpu"])
	resp.Metric = "cpu"
	resp.Server = server
	return resp, nil
}

func (c *Client) Memory(server string) (*MetricsResponse, error) {
	result, err := c.Query(QueryMemory, server)
	if err != nil {
		return nil, err
	}
	resp := c.parseResults(result, thresholds["memory"])
	resp.Metric = "memory"
	resp.Server = server
	return resp, nil
}

func (c *Client) Disk(server string) (*MetricsResponse, error) {
	result, err := c.Query(QueryDisk, server)
	if err != nil {
		return nil, err
	}
	resp := c.parseResults(result, thresholds["disk"])
	resp.Metric = "disk"
	resp.Server = server
	return resp, nil
}

func (c *Client) Load(server string) (*MetricsResponse, error) {
	result, err := c.Query(QueryLoad1, server)
	if err != nil {
		return nil, err
	}
	resp := c.parseResults(result, thresholds["load"])
	resp.Metric = "load"
	resp.Server = server
	return resp, nil
}

func (c *Client) All(server string) (map[string]*MetricsResponse, error) {
	results := make(map[string]*MetricsResponse)

	cpu, err := c.CPU(server)
	if err == nil {
		results["cpu"] = cpu
	}

	memory, err := c.Memory(server)
	if err == nil {
		results["memory"] = memory
	}

	disk, err := c.Disk(server)
	if err == nil {
		results["disk"] = disk
	}

	load, err := c.Load(server)
	if err == nil {
		results["load"] = load
	}

	return results, nil
}

func (c *Client) CustomQuery(query string, server string, threshold float64) (*MetricsResponse, error) {
	result, err := c.Query(query, server)
	if err != nil {
		return nil, err
	}
	resp := c.parseResults(result, threshold)
	resp.Metric = "custom"
	resp.Server = server
	return resp, nil
}

func (c *Client) ListServers() []ServerInfo {
	var servers []ServerInfo
	for name, cfg := range c.config.Servers {
		isDefault := name == c.config.Default
		servers = append(servers, ServerInfo{
			Name:      name,
			URL:       cfg.URL,
			Label:     cfg.Name,
			IsDefault: isDefault,
		})
	}
	return servers
}

type ServerInfo struct {
	Name      string `json:"name"`
	URL       string `json:"url"`
	Label     string `json:"label"`
	IsDefault bool   `json:"is_default"`
}

func (c *Client) AddServer(name, url, label string, setDefault bool) error {
	if c.config.Servers == nil {
		c.config.Servers = make(map[string]ServerConfig)
	}
	c.config.Servers[name] = ServerConfig{
		URL:  url,
		Name: label,
	}
	if setDefault {
		c.config.Default = name
	}
	return c.SaveConfig()
}

func (c *Client) RemoveServer(name string) error {
	if _, ok := c.config.Servers[name]; !ok {
		return fmt.Errorf("server '%s' not found", name)
	}
	delete(c.config.Servers, name)
	if c.config.Default == name && len(c.config.Servers) > 0 {
		for k := range c.config.Servers {
			c.config.Default = k
			break
		}
	}
	return c.SaveConfig()
}

func (c *Client) SetDefault(name string) error {
	if _, ok := c.config.Servers[name]; !ok {
		return fmt.Errorf("server '%s' not found", name)
	}
	c.config.Default = name
	return c.SaveConfig()
}
