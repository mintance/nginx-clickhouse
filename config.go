package main

import (
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"github.com/Sirupsen/logrus"
)

type Config struct {
	Client struct {
		Host string `yaml:"listen_host"`
		Port string `yaml:"listen_port"`
	} `yaml:"client"`
	Cluster struct {
		Host string `yaml:"listen_host"`
		Port string `yaml:"listen_port"`
		AcceptHosts []string `yaml:"accept_hosts"`
	} `yaml:"cluster"`
	System struct {
		ConfigsPath string `yaml:"configs_path"`
	} `yaml:"system"`
	Storage struct {
		Driver string `yaml:"driver"`
		Credentials struct {
			Host string `yaml:"host"`
			Port string `yaml:"port"`
			User string `yaml:"user"`
			Password string `yaml:"password"`
		} `yaml:"credentials"`
	} `yaml:"storage"`
	DockerSecrets struct {
		Path string `yaml:"path"`
	}
	Metrics struct {
		Driver string `yaml:"driver"`
		Credentials struct {
			Host string `yaml:"host"`
			Port string `yaml:"port"`
			User string `yaml:"user"`
			Password string `yaml:"password"`
		} `yaml:"credentials"`
	} `yaml:"metrics"`
	Logs struct {
		Driver string `yaml:"driver"`
		Credentials struct {
			Host string `yaml:"host"`
			Port string `yaml:"port"`
			User string `yaml:"user"`
			Password string `yaml:"password"`
		} `yaml:"credentials"`
	} `yaml:"logs"`
}

const config_path  = `config/config.yaml`

func getConfig() *Config {

	config := Config{}

	logrus.Info("Reading config file: " + config_path)

	data, err := ioutil.ReadFile(config_path)

	if err != nil {
		logrus.Fatal("Config open error: ", err)
	}

	err = yaml.Unmarshal(data, &config)

	if err != nil {
		logrus.Fatal("Config read & unmarshal error: ", err)
	}

	return &config
}
