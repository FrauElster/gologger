# gologger

My small logger library. This is not meant to be used by anyone, I just kept copy pasting this in my projects and thought it is time to have it versioned as dependency. 

It has a global extensible logger, featuring 2 extensions:

- `console`, basically just standard slog
- `loki`, pushing logs in batches to a Loki instance# gologger


## Usage

```go
// with loki
ctx := context.Background()
lokiOpt, err := gologger.WithLoki(ctx, "http://loki:3100/", "eu-1", "my-service", gologger.WithLevels([]slog.Level{slog.LevelError}))
if err != nil {
    panic(err)
}
gologger.Init(slog.LevelWarn, lokiOpt)

gologger.Info("not logged at all")
gologger.Warn("not sent to loki, but printed to stdout")
gologger.Error("sent to loki and stdout")
```

```go
func handleError(err error) {
    if err != nil {
        panic(err)
    }
}

// parsing config
type LogConfig struct {
	Level string
	Loki  string
}

ctx := context.Background()
cfg := LogConfig{Level: "warn", Loki: "http://loki:3100/"}
logLvl, err := gologger.ParseLogLevel(cfg.Level)
handleError(err)
loggerOpts := make([]gologger.Option, 0)
if cfg.Loki != "" {
	lokiOpt, err := gologger.WithLoki(ctx, "http://loki:3100/", "eu-1", "my-service", gologger.WithLevels([]slog.Level{slog.LevelError}))
	handleError(err)
	loggerOpts = append(loggerOpts, lokiOpt)
}
gologger.Init(logLvl, loggerOpts...)

gologger.Info("not logged at all")
gologger.Warn("not sent to loki, but printed to stdout")
gologger.Error("sent to loki and stdout")
```

```go
// with alertmanager
ctx := context.Background()
lokiOpt, err := gologger.WithAlertManager(ctx, "http://loki:3100/", "eu-1", "my-service", gologger.WithLevels([]slog.Level{slog.LevelError}))
if err != nil {
    panic(err)
}
gologger.Init(slog.LevelWarn, lokiOpt)

gologger.Info("not logged at all")
gologger.Warn("not sent to loki, but printed to stdout")
gologger.Error("sent to loki and stdout")
```