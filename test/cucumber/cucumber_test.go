package cucumber

import (
	"flag"
	"os"
	"testing"

	"github.com/cucumber/godog"
	"github.com/cucumber/godog/colors"
)

var opts = godog.Options{
	Output: colors.Colored(os.Stdout),
	Format: "pretty",
}

func init() {
	godog.BindCommandLineFlags("godog.", &opts)
}

func TestMain(_ *testing.M) {
	flag.Parse()

	// Skip cucumber tests in short mode
	if testing.Short() {
		os.Exit(0)
	}

	opts.Paths = flag.Args()

	status := godog.TestSuite{
		Name:                 "redis-operator-integration",
		TestSuiteInitializer: InitializeTestSuite,
		ScenarioInitializer:  InitializeScenario,
		Options:              &opts,
	}.Run()

	os.Exit(status)
}

func TestCucumber(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cucumber integration tests in short mode")
	}

	suite := godog.TestSuite{
		Name:                 "redis-operator-integration",
		TestSuiteInitializer: InitializeTestSuite,
		ScenarioInitializer:  InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fatal("non-zero status returned, failed to run feature tests")
	}
}
