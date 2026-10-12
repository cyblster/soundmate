#!/usr/bin/env bash

exec > >(exec awk -W interactive '
function pad(s, w) { return sprintf("%-" w "s", s) }
function now(   cmd, t) { cmd = "date -u +\"%F %T.%3N\""; cmd | getline t; close(cmd); return t }
function emit(t, msg, attrs) {
	color = level == "INFO" ? "\033[32m" : level == "WARN" ? "\033[33m" : level == "DEBUG" ? "\033[2m" : "\033[31m"
	if (attrs != "") msg = pad(msg, 40) " " attrs
	print "\033[2m" t "\033[0m " color pad(level, 5) "\033[0m \033[34mpostgres \033[0m " msg
	fflush()
}
BEGIN {
	level = "INFO"
	split("DEBUG1 DEBUG2 DEBUG3 DEBUG4 DEBUG5", d, " ")
	for (i in d) levels[d[i]] = "DEBUG"
	levels["LOG"] = "INFO"; levels["INFO"] = "INFO"; levels["NOTICE"] = "INFO"
	levels["WARNING"] = "WARN"; levels["ERROR"] = "ERROR"; levels["FATAL"] = "FATAL"; levels["PANIC"] = "PANIC"
}
/^$/ { next }
match($0, /^[0-9-]+ [0-9:.]+ UTC \[[0-9]+\] [A-Z0-9]+:  /) {
	split(substr($0, 1, RLENGTH), p, " ")
	tag = substr(p[5], 1, length(p[5]) - 1)
	msg = substr($0, RLENGTH + 1)
	if (tag in levels) level = levels[tag]; else msg = tolower(tag) ": " msg
	last = p[1] " " p[2]
	emit(last, msg, "pid=" substr(p[4], 2, length(p[4]) - 2))
	next
}
{ emit(last != "" ? last : now(), $0, "") }
') 2>&1
formatter=$!

if [ "$1" = postgres ]; then
	set -- "$@" -c "log_line_prefix=%m [%p] " -c log_timezone=UTC
fi

docker-entrypoint.sh "$@" &
server=$!
trap 'kill -INT "$server"' INT TERM
status=0
while kill -0 "$server" 2>/dev/null; do
	wait "$server"
	status=$?
done

exec >&- 2>&-
wait "$formatter"
exit "$status"
