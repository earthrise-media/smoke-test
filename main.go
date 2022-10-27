package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/jpillora/backoff"
	iris "github.com/kataras/iris/v12"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"net/http"
	"sync"
	"time"
)

var runMap map[string]bool
var mtx sync.Mutex

func main() {

	preflight()

	app := iris.New()
	app.Get("/health", func(ctx iris.Context) {
		err := ctx.JSON(iris.Map{
			"status": "ok",
		})
		if err != nil {
			zap.S().Errorf(err.Error())
		}
	})
	//old paths
	app.Get("/generate-load", generateLoad)
	app.Post("/generate-load", generateLoad)
	//new ones
	app.Get("/smoke-test", generateLoad)
	app.Post("/smoke-test", generateLoad)

	err := app.Listen(":" + viper.GetString("PORT"))
	if err != nil {
		zap.S().Fatalf(err.Error())
	}

}

func loadUrls(service string) ([]string, error) {

	csvUrl := viper.GetString("REPO_ROOT") + service + ".csv"
	resp, err := http.Get(csvUrl)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("Unable load URLs from %s, check configuration", csvUrl)
	}
	rdr := csv.NewReader(resp.Body)
	//for now we only have 1 field, may add more in the future
	recs, err := rdr.ReadAll()
	if err != nil {
		return nil, err
	}

	urlList := lo.Map[[]string, string](recs, func(s []string, _ int) string { return s[0] })
	return urlList, nil
}

func generateLoad(c iris.Context) {

	if !c.URLParamExists("HOST") || !c.URLParamExists("PROTO") || !c.URLParamExists("PORT") || !c.URLParamExists("DURATION") {
		err := c.Problem(iris.NewProblem().Status(iris.StatusBadRequest).Title("Missing Parameters").Detail("Missing Parameters"))
		if err != nil {
			zap.S().Errorf(err.Error())
		}
		notify(SlackMessage{Text: "Error handling smoke test request: required parameters are missing :x:"})
		return
	}

	var err error
	threads := c.URLParamInt64Default("THREADS", 1)

	target := c.URLParam("PROTO") + "://" + c.URLParam("HOST") + ":" + c.URLParam("PORT")

	dataset := c.URLParamDefault("SERVICE", "api-v3")

	urls, err := loadUrls(dataset)
	if err != nil {
		err2 := c.Problem(iris.NewProblem().Status(iris.StatusBadRequest).Title("Invalid service").Detail("Can't find url file: " + err.Error()))
		if err2 != nil {
			zap.S().Errorf(err2.Error())
		}

		notify(SlackMessage{Text: fmt.Sprintf("Error handling smoke test request: can't load URLs for %s service :x:", dataset)})
		return
	}
	dur, err := time.ParseDuration(c.URLParam("DURATION"))
	if err != nil {
		err2 := c.Problem(iris.NewProblem().Status(iris.StatusBadRequest).Title("Invalid Duration").Detail("Can't parse duration: " + err.Error()))
		if err2 != nil {
			zap.S().Errorf(err2.Error())
		}
		notify(SlackMessage{Text: fmt.Sprintf("Error handling smoke test request: unable to parse duration value %s :x:", err.Error())})
		return
	}

	//debounce
	mtx.Lock()
	defer mtx.Unlock()
	ok, runnning := runMap[target];
	if ok {
		if runnning {
			err = c.JSON(iris.Map{"status": fmt.Sprintf("already running against %s", target)})
			if err != nil {
				zap.S().Error(err.Error())
			}
			c.EndRequest()
			return
		} else {
			runMap[target] = true
		}
	} else {
		runMap[target] = true
	}
	go func() {
		timer := time.NewTimer(dur)
		ctx, cancel := context.WithCancel(context.Background())
		//notify slack of start
		notify(SlackMessage{Text: fmt.Sprintf("Starting smoke-test run against %s service running on %s", dataset, target)})
		// launch a goroutine for each "thread" requested
		for i := int64(0); i < threads; i++ {
			go pollUrls(urls, target, ctx)
		}
		//let it run until the timer expires
		<-timer.C
		mtx.Lock()
		defer mtx.Unlock()
		runMap[target] = false
		cancel()
	}()

	err = c.JSON(iris.Map{
		"status": "ok",
	})
	if err != nil {
		zap.S().Error(err.Error())
	}
	c.EndRequest()
}

func pollUrls(urls []string, target string, ctx context.Context) {
	var success, warnings, errors = 0, 0, 0
	bo := backoff.Backoff{
		Min:    1 * time.Second,
		Max:    10 * time.Second,
		Factor: 2,
		Jitter: true,
	}
	i := 0
	for {
		if ctx.Err() != nil {
			zap.S().Infof("completed with %d successes %d warnings and %d errors", success, warnings, errors)
			notify(SlackMessage{Text: fmt.Sprintf("Finished smoke-test run against %s service: \n %d successes :white_check_mark: \n %d warnings :warning: \n %d errors :x: ", target, success, warnings, errors)})
			return
		}
		path := urls[i]
		resp, err := http.Get(target + path)
		if err != nil {
			zap.S().Error(err.Error())
		} else {
			if resp.StatusCode < 299 {
				zap.S().Warnf("%s -- Code: %d Length: %d Content-Type: %s \u2714", path, resp.StatusCode, resp.ContentLength, resp.Header.Get("Content-Type"))
				success++
			} else if resp.StatusCode > 399 && resp.StatusCode < 500 {
				out := fmt.Sprintf("%s -- Code: %d \u26a0", path, resp.StatusCode)
				notify(SlackMessage{Text: out})
				zap.S().Warn(out)
				warnings++
				time.Sleep(bo.Duration())
			} else if resp.StatusCode > 499 && resp.StatusCode < 600 {
				out := fmt.Sprintf("%s -- Code: %d \u274c", path, resp.StatusCode)
				notify(SlackMessage{Text: out})
				zap.S().Warn(out)

				errors++
				time.Sleep(bo.Duration())
			} else {
				zap.S().Warnf("%s -- Code: %d", path, resp.StatusCode)
				time.Sleep(bo.Duration())
			}
		}
		if i == len(urls)-1 {
			i = 0
			continue
		}
		i++
	}
}

func preflight() {

	viper.SetDefault("PORT", 8001)
	viper.SetDefault("LOG_LEVEL", "INFO")
	viper.SetDefault("JSON_LOG", false)
	viper.SetDefault("VERBOSE", true)
	viper.SetDefault("REPO_ROOT", "https://raw.githubusercontent.com/earthrise-media/smoke-test-urls/main/")
	viper.SetDefault("SLACK_TOKEN", "")
	viper.SetDefault("SLACK_CHANNEL", "trace-notifications")
	viper.AutomaticEnv()

	//setup logging
	loggerConfig := zap.NewProductionConfig()
	loggerConfig.Sampling = nil
	err := loggerConfig.Level.UnmarshalText([]byte(viper.GetString("LOG_LEVEL")))
	if err != nil {
		zap.S().Errorf("unable to set log level: %s", err)
	}
	loggerConfig.EncoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	loggerConfig.EncoderConfig.TimeKey = "ts"
	loggerConfig.EncoderConfig.LevelKey = "l"
	loggerConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	loggerConfig.OutputPaths = []string{"stdout"}
	loggerConfig.ErrorOutputPaths = []string{"stderr"}
	logger, _ := loggerConfig.Build()
	zap.ReplaceGlobals(logger)

}

func notify(message SlackMessage) {
	message.Channel = viper.GetString("SLACK_CHANNEL")
	mess, err := json.Marshal(message)
	if err != nil {
		zap.S().Errorf(err.Error())
		return
	}
	data := bytes.NewBuffer(mess)
	req, err := http.NewRequest("POST", "https://slack.com/api/chat.postMessage", data)
	if err != nil {
		//well shit...
		zap.S().Errorf(err.Error())
		return
	}
	req.Header.Add("Authorization", "Bearer "+viper.GetString("SLACK_TOKEN"))
	req.Header.Add("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		//well shit...
		zap.S().Errorf(err.Error())
		return
	}
	zap.S().Infof(resp.Status)
}

type SlackMessage struct {
	Channel string `json:"channel"`
	Text    string `json:"text"`
}
