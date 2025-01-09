// Copyright 2025 Redpanda Data, Inc.

package log

import (
	"github.com/redpanda-data/benthos/v4/internal/docs"
)

// Spec returns a field spec for the logger configuration fields.
func Spec() docs.FieldSpecs {
	return docs.FieldSpecs{
		docs.FieldString(fieldLogLevel, "Set the minimum severity level for emitting logs.").HasOptions(
			"OFF", "FATAL", "ERROR", "WARN", "INFO", "DEBUG", "TRACE", "ALL", "NONE",
		).HasDefault("INFO").LinterFunc(nil),
		docs.FieldString(fieldFormat, "Set the format of emitted logs.").HasOptions("json", "logfmt").HasDefault("logfmt"),
		docs.FieldBool(fieldAddTimeStamp, "Whether to include timestamps in logs.").HasDefault(false),
		docs.FieldString(fieldLevelName, "The name of the level field added to logs when the `format` is `json`.").HasDefault("level").Advanced(),
		docs.FieldString(fieldTimestampName, "The name of the timestamp field added to logs when `add_timestamp` is set to `true` and the `format` is `json`.").HasDefault("time").Advanced(),
		docs.FieldString(fieldMessageName, "The name of the message field added to logs when the `format` is `json`.").HasDefault("msg").Advanced(),
		docs.FieldString(fieldStaticFields, "A map of key/value pairs to add to each structured log.").Map().HasDefault(map[string]any{
			"@service": "benthos",
		}),
		docs.FieldObject(fieldFile, "Experimental: Specify fields for optionally writing logs to a file.").WithChildren(
			docs.FieldString(fieldFilePath, "The file path to write logs to, if the file does not exist it will be created. Leave this field empty or unset to disable file based logging.").HasDefault(""),
			docs.FieldBool(fieldFileRotate, "Whether to rotate log files automatically.").HasDefault(false),
			docs.FieldInt(fieldFileRotateMaxAge, "The maximum number of days to retain old log files based on the timestamp encoded in their filename, after which they are deleted. Setting to zero disables this mechanism.").HasDefault(0),
		).Advanced(),
	}
}
