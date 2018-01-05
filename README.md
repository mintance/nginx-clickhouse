# nginx-clickhouse &nbsp; [![Tweet](https://img.shields.io/twitter/url/http/shields.io.svg?style=social)](https://twitter.com/intent/tweet?text=Simple%20NGINX%20logs%20parser%20and%20transporter%20to%20ClickHouse%20database.%20&amp;url=https://github.com/mintance/nginx-clickhouse&amp;hashtags=nginx,clickhouse,golang)

[![License: Apache 2](https://img.shields.io/hexpm/l/plug.svg)](https://github.com/mintance/nginx-clickhouse/blob/master/LICENSE)
![Golang Version](https://img.shields.io/badge/golang-1.5%2B-blue.svg)
[![Docker Build Status](https://img.shields.io/docker/build/mintance/nginx-clickhouse.svg)](https://hub.docker.com/r/mintance/nginx-clickhouse/)
[![Docker Pulls](https://img.shields.io/docker/pulls/mintance/nginx-clickhouse.svg)](https://hub.docker.com/r/mintance/nginx-clickhouse/)
[![Docker Stars](https://img.shields.io/docker/stars/mintance/nginx-clickhouse.svg)](https://hub.docker.com/r/mintance/nginx-clickhouse/)
[![GitHub issues](https://img.shields.io/github/issues/mintance/nginx-clickhouse.svg)](https://github.com/mintance/nginx-clickhouse/issues)

Simple nginx logs parser &amp; transporter to ClickHouse database.

### How to build from sources

#### 1. Install helpers

`make install-helpers`

#### 2. Install dependencies

`make dependencies`

#### 3. Build binary file

`make build`

### How to build Docker image

To build image just type this command, and it will compile binary from sources and create Docker image. You don't need to have Go development tools, the [build process will be in Docker](https://medium.com/travis-on-docker/multi-stage-docker-builds-for-creating-tiny-go-images-e0e1867efe5a).

`make docker`

### How to run

#### 1. Pull image from Docker Hub (or build from sources)

`docker pull mintance/nginx-clickhouse`

There are always last stable image, it automatically builds when release created.

#### 2. Run Docker container

For this example, we include `/var/log/nginx` directory, where we store our logs, and `config` directory where we store `config.yml` file.

`docker run --rm --net=host --name nginx-clickhouse -v /var/log/nginx:/logs -v config:/config -d mintance/nginx-clickhouse`

### How it works?

Here are described full setting-up example.

#### NGINX log format description

In nginx, there are: [nginx_http_log_module](http://nginx.org/en/docs/http/ngx_http_log_module.html) that writes request logs in the specified format.

They are defined in `/etc/nginx/nginx.conf` file. For example we create `main` log format.

```lua
http {
    ...
     log_format main '$remote_addr - $remote_user [$time_local]
         "$request" $status $bytes_sent "$http_referer" "$http_user_agent"';
    ...
}
```

After defining this, we can use it in our site config `/etc/nginx/sites-enabled/my-site.conf` inside server section:

```lua
server {
  ...
  access_log /var/log/nginx/my-site-access.log main;
  ...
}
```

Now all what we need, is to create `config.yml` file where we describe our log format, log file path, and ClickHouse credentials. We can also use environment variables for this.

#### ClickHouse table schema example

This is table schema for our example.

```sql
CREATE TABLE metrics.nginx (
    RemoteAddr String,
    RemoteUser String,
    TimeLocal DateTime,
    Date Date DEFAULT toDate(TimeLocal),
    Request String,
    RequestMethod String,
    Status Int32,
    BytesSent Int64,
    HttpReferer String,
    HttpUserAgent String,
    RequestTime Float32,
    UpstreamConnectTime Float32,
    UpstreamHeaderTime Float32,
    UpstreamResponseTime Float32,
    Https FixedString(2),
    ConnectionsWaiting Int64,
    ConnectionsActive Int64
) ENGINE = MergeTree(Date, (Status, Date), 8192)
```

#### Config file description

##### 1. Log path & flushing interval

```yaml
settings:
  interval: 5 # in seconds
  log_path: /var/log/nginx/my-site-access.log # path to logfile
```

##### 2. ClickHouse credentials and table schema

```yaml
clickhouse:
 db: metrics # Database name
 table: nginx # Table name
 host: localhost # ClickHouse host (cluster support will be added later)
 port: 8123 # ClicHhouse HTTP port
 credentials:
  user: default # User name
  password: # User password
```

Here we describe in key-value format (key - ClickHouse column, value - log variable) relation between column and log variable.

```yaml
columns:
    RemoteAddr: remote_addr
    RemoteUser: remote_user
    TimeLocal: time_local
    Request: request
    Status: status
    BytesSent: bytes_sent
    HttpReferer: http_referer
    HttpUserAgent: http_user_agent
```

##### 3. NGINX log type & format

In `log_format` - we just copy format from nginx.conf

```yaml
nginx:
  log_type: main
  log_format: $remote_addr - $remote_user [$time_local] "$request" $status $bytes_sent "$http_referer" "$http_user_agent"
```

##### 4. Full config file example

```yaml
settings:
    interval: 5
    log_path: /var/log/nginx/my-site-access.log
clickhouse:
    db: metrics
    table: nginx
    host: localhost
    port: 8123
    credentials:
        user: default
        password:
    columns:
        RemoteAddr: remote_addr
        RemoteUser: remote_user
        TimeLocal: time_local
        Request: request
        Status: status
        BytesSent: bytes_sent
        HttpReferer: http_referer
        HttpUserAgent: http_user_agent
nginx:
    log_type: main
    log_format: $remote_addr - $remote_user [$time_local] "$request" $status $bytes_sent "$http_referer" "$http_user_agent"
```

#### Grafana Dashboard

After all steps you can build your own grafana dashboards.

![alt text](https://github.com/mintance/nginx-clickhouse/blob/master/grafana.png)

![alt text](https://github.com/openbsod/nginx2clickhouse/blob/master/iptv-status-returned.png)
