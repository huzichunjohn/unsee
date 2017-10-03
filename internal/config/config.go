package config

import (
	"bytes"
	"flag"
	"fmt"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"
	"unicode"

	"github.com/kelseyhightower/envconfig"

	log "github.com/sirupsen/logrus"
)

type spaceSeparatedList []string

func (mvd *spaceSeparatedList) Decode(value string) error {
	*mvd = spaceSeparatedList(strings.Split(value, " "))
	return nil
}

type configEnvs struct {
	AlertmanagerTimeout      time.Duration      `envconfig:"ALERTMANAGER_TIMEOUT" default:"40s" help:"Timeout for all request send to Alertmanager"`
	AlertmanagerTTL          time.Duration      `envconfig:"ALERTMANAGER_TTL" default:"1m" help:"TTL for Alertmanager alerts and silences"`
	AlertmanagerURIs         spaceSeparatedList `envconfig:"ALERTMANAGER_URIS" required:"true" help:"List of Alertmanager URIs (name:uri)"`
	AnnotationsHidden        spaceSeparatedList `envconfig:"ANNOTATIONS_HIDDEN" help:"List of annotations that are hidden by default"`
	AnnotationsDefaultHidden bool               `envconfig:"ANNOTATIONS_DEFAULT_HIDDEN" default:"false" help:"Hide all annotations by default unless listed in ANNOTATIONS_VISIBLE"`
	AnnotationsVisible       spaceSeparatedList `envconfig:"ANNOTATIONS_VISIBLE" help:"List of annotations that are visible by default"`
	ColorLabelsStatic        spaceSeparatedList `envconfig:"COLOR_LABELS_STATIC" help:"List of label names that should have the same (but distinct) color"`
	ColorLabelsUnique        spaceSeparatedList `envconfig:"COLOR_LABELS_UNIQUE" help:"List of label names that should have unique color"`
	ConfigFile               string             `envconfig:"CONFIG_FILE" help:"Path to configuration file"`
	Debug                    bool               `envconfig:"DEBUG" default:"false" help:"Enable debug mode"`
	FilterDefault            string             `envconfig:"FILTER_DEFAULT" help:"Default filter string"`
	JiraRegexp               spaceSeparatedList `envconfig:"JIRA_REGEX" help:"List of JIRA regex rules"`
	Port                     int                `envconfig:"PORT" default:"8080" help:"HTTP port to listen on"`
	SentryDSN                string             `envconfig:"SENTRY_DSN" help:"Sentry DSN for Go exceptions"`
	SentryPublicDSN          string             `envconfig:"SENTRY_PUBLIC_DSN" help:"Sentry DSN for javascript exceptions"`
	StripLabels              spaceSeparatedList `envconfig:"STRIP_LABELS" help:"List of labels to ignore"`
	KeepLabels               spaceSeparatedList `envconfig:"KEEP_LABELS" help:"List of labels to keep, all other labels will be stripped"`
	WebPrefix                string             `envconfig:"WEB_PREFIX" default:"/" help:"URL prefix"`
}

type configYAML struct {
	Alertmanagers []struct {
		URI     string        `yaml:"uri"`
		Timeout time.Duration `yaml:"timeout"`
	} `yaml:"alertmanagers"`
	AlertmanagerTTL time.Duration `yaml:"ttl"`
	Annotations     struct {
		DefaultHidden bool     `yaml:"default_hidden"`
		Hidden        []string `yaml:"hidden"`
		Visible       []string `yaml:"visible"`
	} `yaml:"annotations"`
	Colors struct {
		Labels struct {
			Static []string `yaml:"static"`
			Unique []string `yaml:"unique"`
		} `yaml:"labels"`
	} `yaml:"colors"`
	Debug  bool `yaml:"debug"`
	Labels struct {
		Strip []string `yaml:"strip"`
		Keep  []string `yaml:"keep"`
	} `yaml:"labels"`
	Listen struct {
		Address string `yaml:"address"`
		Port    int    `yaml:"port"`
		Prefix  string `yaml:"prefix"`
	} `yaml:"listen"`
	Filter string `yaml:"filter"`
	JIRA   []struct {
		Rule string `yaml:"rule"`
		URI  string `yaml:"uri"`
	} `yaml:"jira"`
	Sentry struct {
		Private string `yaml:"private"`
		Public  string `yaml:"public"`
	} `yaml:"sentry"`
}

// Config exposes all options required to run
var Config configEnvs

// generate flag name from the option name, a dot will be injected between
// <lower case char><upper case char>
func makeFlagName(s string) string {
	var buffer bytes.Buffer
	prevUpper := true
	for _, rune := range s {
		if unicode.IsUpper(rune) && !prevUpper {
			buffer.WriteRune('.')
		}
		prevUpper = unicode.IsUpper(rune)
		buffer.WriteRune(unicode.ToLower(rune))
	}
	return buffer.String()
}

// Iterate all defined envconfig variables and generate a flag for each key.
// Next parse those flags and for each set flag inject env variable which will
// be read by envconfig later on.
type flagMapper struct {
	isBool    bool
	stringVal *string
	boolVal   *bool
}

func mapEnvConfigToFlags() {
	flags := make(map[string]flagMapper)
	s := reflect.ValueOf(Config)
	typeOfSpec := s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := typeOfSpec.Field(i)

		flagName := makeFlagName(f.Name)
		// check if flag was already set, this usually happens only during testing
		if flag.Lookup(flagName) != nil {
			continue
		}

		envName := f.Tag.Get("envconfig")

		helpMsg := fmt.Sprintf("%s. This flag can also be set via %s environment variable.", f.Tag.Get("help"), f.Tag.Get("envconfig"))
		if f.Tag.Get("required") == "true" {
			helpMsg = fmt.Sprintf("%s This option is required.", helpMsg)
		}

		mapper := flagMapper{}
		if s.Field(i).Kind() == reflect.Bool {
			mapper.isBool = true
			mapper.boolVal = flag.Bool(flagName, false, helpMsg)
		} else {
			mapper.stringVal = flag.String(flagName, "", helpMsg)
		}
		flags[envName] = mapper
	}
	flag.Parse()
	for envName, mapper := range flags {
		if mapper.isBool {
			if *mapper.boolVal == true {
				err := os.Setenv(envName, "true")
				if err != nil {
					log.Fatal(err)
				}
			}
		} else {
			if *mapper.stringVal != "" {
				err := os.Setenv(envName, *mapper.stringVal)
				if err != nil {
					log.Fatal(err)
				}
			}
		}
	}
}

func (config *configEnvs) Read() {
	mapEnvConfigToFlags()

	err := envconfig.Process("", config)
	if err != nil {
		log.Fatal(err)
	}
}

func hideURLPassword(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	if u.User != nil {
		if _, pwdSet := u.User.Password(); pwdSet {
			u.User = url.UserPassword(u.User.Username(), "xxx")
		}
		return u.String()
	}
	return s
}

func (config *configEnvs) LogValues() {
	s := reflect.ValueOf(config).Elem()
	typeOfT := s.Type()
	for i := 0; i < s.NumField(); i++ {
		env := typeOfT.Field(i).Tag.Get("envconfig")
		val := fmt.Sprintf("%v", s.Field(i).Interface())
		log.Infof("%20s => %v", env, hideURLPassword(val))
	}

}
