# gologger

My small logger library. This is not meant to be used by anyone, I just kept copy pasting this in my projects and thought it is time to have it versioned as dependency.

It has a global extensible logger, featuring 4 extensions:

- `console`, basically just standard slog
- `loki`, pushing logs in batches to a Loki instance# gologger
- `file`, writing logs to a file
- `database`, writing logs to a database

## Usage

```go
import "github.com/FrauElster/gologger/v2"

// with loki
if err := gologger.Setup("warn"); err != nil {
	return err
}

if lokiURL != "" {
	errLevel, _ := gologger.ParseLevel("error")
	err := gologger.UseLoki(gologger.LokiConfig{
		URL:       lokiURL,
		BatchWait: 5*time.Second,
		Labels: map[string]string{"source":  "myapp","version": "1.0"},
		MinLevel: &errLevel,
	})
	if err != nil {
		return err
	}
}

if logFile != "" {
	err := logger.UseFile(gologger.FileConfig{Path: logFile, LabelsMap: map[string]string{"source": "myApp", "version": "1.0"}})
	if err != nil {
		return err
	}
}

var db *sql.DB // your db connection
if db != nil {
	infoLevel, _ := gologger.ParseLevel("info")
	err := logger.UseSqlite(gologger.DbConfig{DB: db, TableName: "logs",LabelsMap: map[string]string{"source": "myApp", "version": "1.0"}, MinLevel: &infoLevel})
	if err != nil {
		return err
	}
}

gologger.Info("not logged at all")
gologger.Warn("written to stdout, file, and database, but not to loki", "key", "value")
gologger.Error("sent to loki and stdout")
```
