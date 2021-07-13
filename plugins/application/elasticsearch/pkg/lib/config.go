package lib

// AppConfig holds configuration for Elasticsearch client
type AppConfig struct {
	HostURL       string   `yaml:"hostURL"`
	UseTLS        bool     `yaml:"useTLS"`
	TLSServerName string   `yaml:"tlsServerName"`
	TLSClientCert string   `yaml:"tlsClientCert"`
	TLSClientKey  string   `yaml:"tlsClientKey"`
	TLSCaCert     string   `yaml:"tlsCaCert"`
	UseBasicAuth  bool     `yaml:"useBasicAuth"`
	User          string   `yaml:"user"`
	Password      string   `yaml:"password"`
	BufferSize    int      `yaml:"bufferSize"`
	BulkIndex     bool     `yaml:"bulkIndex"`
	ResetIndices  []string `yaml:"resetIndices"`
	IndexWorkers  int      `yaml:"indexWorkers"`
}
