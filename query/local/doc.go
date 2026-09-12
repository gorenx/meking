// Package local answers questions from an independently fixed Entity vector
// view and one immutable ReportSet publication. It resolves every selected
// Knowledge row by exact logical ID and Version before constructing model
// context, so a later Knowledge update cannot rewrite an in-flight query.
package local
