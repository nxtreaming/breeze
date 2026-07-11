package main

import (
	"flag"
	"reflect"
	"testing"
)

func testFlagSet() *flag.FlagSet {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("plural", "", "")
	fs.Bool("force", false, "")
	return fs
}

func TestSplitFlagsEqualsForm(t *testing.T) {
	flagArgs, positional := splitFlagsAndPositional(testFlagSet(), []string{"name:string", "--plural=people", "--force", "email:string"})
	if want := []string{"--plural=people", "--force"}; !reflect.DeepEqual(flagArgs, want) {
		t.Errorf("flagArgs = %v, want %v", flagArgs, want)
	}
	if want := []string{"name:string", "email:string"}; !reflect.DeepEqual(positional, want) {
		t.Errorf("positional = %v, want %v", positional, want)
	}
}

func TestSplitFlagsSpaceSeparatedValue(t *testing.T) {
	flagArgs, positional := splitFlagsAndPositional(testFlagSet(), []string{"name:string", "--plural", "people", "email:string"})
	if want := []string{"--plural", "people"}; !reflect.DeepEqual(flagArgs, want) {
		t.Errorf("flagArgs = %v, want %v", flagArgs, want)
	}
	if want := []string{"name:string", "email:string"}; !reflect.DeepEqual(positional, want) {
		t.Errorf("positional = %v, want %v", positional, want)
	}
}

func TestSplitFlagsBoolDoesNotConsumeValue(t *testing.T) {
	flagArgs, positional := splitFlagsAndPositional(testFlagSet(), []string{"--force", "name:string"})
	if want := []string{"--force"}; !reflect.DeepEqual(flagArgs, want) {
		t.Errorf("flagArgs = %v, want %v", flagArgs, want)
	}
	if want := []string{"name:string"}; !reflect.DeepEqual(positional, want) {
		t.Errorf("positional = %v, want %v", positional, want)
	}
}

func TestSplitFlagsUnknownFlagKeptForParseError(t *testing.T) {
	flagArgs, _ := splitFlagsAndPositional(testFlagSet(), []string{"--bogus", "name:string"})
	if want := []string{"--bogus"}; !reflect.DeepEqual(flagArgs, want) {
		t.Errorf("flagArgs = %v, want %v", flagArgs, want)
	}
}

func TestUnknownFlagReturnsError(t *testing.T) {
	if err := runNew([]string{"--bogus", "myapp"}); err == nil {
		t.Error("runNew with unknown flag: expected error, got nil")
	}
	if err := generateResource("example.com/x", "User", []string{"--bogus", "name:string"}); err == nil {
		t.Error("generateResource with unknown flag: expected error, got nil")
	}
	if err := generateHandler("example.com/x", "User", []string{"--bogus"}); err == nil {
		t.Error("generateHandler with unknown flag: expected error, got nil")
	}
}
