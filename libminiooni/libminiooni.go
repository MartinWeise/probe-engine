// Package libminiooni implements the cmd/miniooni CLI.
//
// This CLI is compatible with both OONI Probe v2.x and MK v0.10.x.
package libminiooni

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/apex/log"
	engine "github.com/ooni/probe-engine"
	"github.com/ooni/probe-engine/internal/humanizex"
	"github.com/ooni/probe-engine/version"
	"github.com/pborman/getopt/v2"
)

// Options contains the options you can set from the CLI.
type Options struct {
	Annotations  []string
	BouncerURL   string
	CollectorURL string
	Inputs       []string
	ExtraOptions []string
	NoBouncer    bool
	NoGeoIP      bool
	NoJSON       bool
	NoCollector  bool
	Proxy        string
	ReportFile   string
	Verbose      bool
}

const (
	softwareName    = "miniooni"
	softwareVersion = version.Version
)

var (
	globalOptions Options
	startTime     = time.Now()
)

func init() {
	getopt.FlagLong(
		&globalOptions.Annotations, "annotation", 'A', "Add annotaton", "KEY=VALUE",
	)
	getopt.FlagLong(
		&globalOptions.BouncerURL, "bouncer", 'b', "Set bouncer base URL", "URL",
	)
	getopt.FlagLong(
		&globalOptions.CollectorURL, "collector", 'c',
		"Set collector base URL", "URL",
	)
	getopt.FlagLong(
		&globalOptions.Inputs, "input", 'i',
		"Add test-dependent input to the test input", "INPUT",
	)
	getopt.FlagLong(
		&globalOptions.ExtraOptions, "option", 'O',
		"Pass an option to the experiment", "KEY=VALUE",
	)
	getopt.FlagLong(
		&globalOptions.NoBouncer, "no-bouncer", 0, "Don't use the OONI bouncer",
	)
	getopt.FlagLong(
		&globalOptions.NoGeoIP, "no-geoip", 'g',
		"Disable GeoIP lookup (not implemented!)",
	)
	getopt.FlagLong(
		&globalOptions.NoJSON, "no-json", 'N', "Disable writing to disk",
	)
	getopt.FlagLong(
		&globalOptions.NoCollector, "no-collector", 'n', "Don't use a collector",
	)
	getopt.FlagLong(
		&globalOptions.Proxy, "proxy", 'P', "Set the proxy URL", "URL",
	)
	getopt.FlagLong(
		&globalOptions.ReportFile, "reportfile", 'o',
		"Set the report file path", "PATH",
	)
	getopt.FlagLong(
		&globalOptions.Verbose, "verbose", 'v', "Increase verbosity",
	)
}

func fatalWithString(msg string) {
	panic(msg)
}

func fatalIfFalse(cond bool, msg string) {
	if !cond {
		log.Warn(msg)
		panic(msg)
	}
}

// Main is the main function of miniooni. This function parses the command line
// options and uses a global state. Use MainWithConfiguration if you want to avoid
// using any global state and relying on command line options.
//
// This function will panic in case of a fatal error. It is up to you that
// integrate this function to either handle the panic of ignore it.
func Main() {
	getopt.Parse()
	fatalIfFalse(len(getopt.Args()) == 1, "Missing experiment name")
	MainWithConfiguration(getopt.Arg(0), globalOptions)
}

func split(s string) (string, string, error) {
	v := strings.SplitN(s, "=", 2)
	if len(v) != 2 {
		return "", "", errors.New("invalid key-value pair")
	}
	return v[0], v[1], nil
}

func fatalOnError(err error, msg string) {
	if err != nil {
		log.WithError(err).Warn(msg)
		panic(msg)
	}
}

func warnOnError(err error, msg string) {
	if err != nil {
		log.WithError(err).Warn(msg)
	}
}

func mustMakeMap(input []string) (output map[string]string) {
	output = make(map[string]string)
	for _, opt := range input {
		key, value, err := split(opt)
		fatalOnError(err, "cannot split key-value pair")
		output[key] = value
	}
	return
}

func mustParseURL(URL string) *url.URL {
	rv, err := url.Parse(URL)
	fatalOnError(err, "cannot parse URL")
	return rv
}

type logHandler struct {
	io.Writer
}

func (h *logHandler) HandleLog(e *log.Entry) (err error) {
	s := fmt.Sprintf("[%14.6f] <%s> %s", time.Since(startTime).Seconds(), e.Level, e.Message)
	if len(e.Fields) > 0 {
		s += fmt.Sprintf(": %+v", e.Fields)
	}
	s += "\n"
	_, err = h.Writer.Write([]byte(s))
	return
}

// See https://gist.github.com/miguelmota/f30a04a6d64bd52d7ab59ea8d95e54da
func gethomedir() string {
	if runtime.GOOS == "windows" {
		home := os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
		if home == "" {
			home = os.Getenv("USERPROFILE")
		}
		return home
	}
	if runtime.GOOS == "linux" {
		home := os.Getenv("XDG_CONFIG_HOME")
		if home != "" {
			return home
		}
		// fallthrough
	}
	return os.Getenv("HOME")
}

// MainWithConfiguration is the miniooni main with a specific configuration
// represented by the experiment name and the current options.
//
// This function will panic in case of a fatal error. It is up to you that
// integrate this function to either handle the panic of ignore it.
func MainWithConfiguration(experimentName string, currentOptions Options) {
	extraOptions := mustMakeMap(currentOptions.ExtraOptions)
	annotations := mustMakeMap(currentOptions.Annotations)

	logger := &log.Logger{Level: log.InfoLevel, Handler: &logHandler{Writer: os.Stderr}}
	if currentOptions.Verbose {
		logger.Level = log.DebugLevel
	}
	if currentOptions.ReportFile == "" {
		currentOptions.ReportFile = "report.jsonl"
	}
	log.Log = logger

	homeDir := gethomedir()
	fatalIfFalse(homeDir != "", "home directory is empty")
	miniooniDir := path.Join(homeDir, ".miniooni")
	assetsDir := path.Join(miniooniDir, "assets")
	err := os.MkdirAll(assetsDir, 0700)
	fatalOnError(err, "cannot create assets directory")
	log.Infof("miniooni state directory: %s", miniooniDir)
	tempDir, err := ioutil.TempDir("", "miniooni")
	fatalOnError(err, "cannot get a temporary directory")
	log.Debugf("miniooni temporary directory: %s", tempDir)

	var proxyURL *url.URL
	if currentOptions.Proxy != "" {
		proxyURL = mustParseURL(currentOptions.Proxy)
	}

	kvstore2dir := filepath.Join(miniooniDir, "kvstore2")
	kvstore, err := engine.NewFileSystemKVStore(kvstore2dir)
	fatalOnError(err, "cannot create kvstore2 directory")

	sess, err := engine.NewSession(engine.SessionConfig{
		AssetsDir:       assetsDir,
		KVStore:         kvstore,
		Logger:          logger,
		ProxyURL:        proxyURL,
		SoftwareName:    softwareName,
		SoftwareVersion: softwareVersion,
		TempDir:         tempDir,
	})
	fatalOnError(err, "cannot create measurement session")
	defer func() {
		sess.Close()
		log.Infof("whole session: recv %s, sent %s",
			humanizex.SI(sess.KibiBytesReceived()*1024, "byte"),
			humanizex.SI(sess.KibiBytesSent()*1024, "byte"),
		)
	}()

	if currentOptions.BouncerURL != "" {
		sess.AddAvailableHTTPSBouncer(currentOptions.BouncerURL)
	}
	if currentOptions.CollectorURL != "" {
		// Implementation note: setting the collector before doing the lookup
		// is totally fine because it's a maybe lookup, meaning that any bit
		// of information already available will not be looked up again.
		sess.AddAvailableHTTPSCollector(currentOptions.CollectorURL)
	}

	if !currentOptions.NoBouncer {
		log.Info("Looking up OONI backends; please be patient...")
		err := sess.MaybeLookupBackends()
		fatalOnError(err, "cannot lookup OONI backends")
	}
	// See https://github.com/ooni/probe-engine/issues/297
	fatalIfFalse(
		currentOptions.NoGeoIP == false,
		"Sorry, the -g option is not implemented.",
	)
	log.Info("Looking up your location; please be patient...")
	err = sess.MaybeLookupLocation()
	fatalOnError(err, "cannot lookup your location")
	log.Infof("- IP: %s", sess.ProbeIP())
	log.Infof("- country: %s", sess.ProbeCC())
	log.Infof("- network: %s (%s)", sess.ProbeNetworkName(), sess.ProbeASNString())
	log.Infof("- resolver's IP: %s", sess.ResolverIP())
	log.Infof("- resolver's network: %s (%s)", sess.ResolverNetworkName(),
		sess.ResolverASNString())

	builder, err := sess.NewExperimentBuilder(experimentName)
	fatalOnError(err, "cannot create experiment builder")
	if builder.NeedsInput() {
		if len(currentOptions.Inputs) <= 0 {
			log.Info("Fetching test lists")
			list, err := sess.QueryTestListsURLs(&engine.TestListsURLsConfig{
				Limit: 16,
			})
			fatalOnError(err, "cannot fetch test lists")
			for _, entry := range list.Result {
				currentOptions.Inputs = append(currentOptions.Inputs, entry.URL)
			}
		}
	} else if len(currentOptions.Inputs) != 0 {
		fatalWithString("this experiment does not expect any input")
	} else {
		// Tests that do not expect input internally require an empty input to run
		currentOptions.Inputs = append(currentOptions.Inputs, "")
	}
	for key, value := range extraOptions {
		if value == "true" || value == "false" {
			err := builder.SetOptionBool(key, value == "true")
			fatalOnError(err, "cannot set boolean option")
		} else {
			err := builder.SetOptionString(key, value)
			fatalOnError(err, "cannot set string option")
		}
	}
	experiment := builder.NewExperiment()
	defer func() {
		log.Infof("experiment: recv %s, sent %s",
			humanizex.SI(experiment.KibiBytesReceived()*1024, "byte"),
			humanizex.SI(experiment.KibiBytesSent()*1024, "byte"),
		)
	}()

	if !currentOptions.NoCollector {
		log.Info("Opening report; please be patient...")
		err := experiment.OpenReport()
		fatalOnError(err, "cannot open report")
		defer experiment.CloseReport()
		log.Infof("Report ID: %s", experiment.ReportID())
	}

	inputCount := len(currentOptions.Inputs)
	inputCounter := 0
	for _, input := range currentOptions.Inputs {
		inputCounter++
		if input != "" {
			log.Infof("[%d/%d] running with input: %s", inputCounter, inputCount, input)
		}
		measurement, err := experiment.Measure(input)
		warnOnError(err, "measurement failed")
		measurement.AddAnnotations(annotations)
		if !currentOptions.NoCollector {
			log.Infof("submitting measurement to OONI collector; please be patient...")
			err := experiment.SubmitAndUpdateMeasurement(measurement)
			warnOnError(err, "submitting measurement failed")
		}
		if !currentOptions.NoJSON {
			// Note: must be after submission because submission modifies
			// the measurement to include the report ID.
			log.Infof("saving measurement to disk")
			err := experiment.SaveMeasurement(measurement, currentOptions.ReportFile)
			warnOnError(err, "saving measurement failed")
		}
	}
}
