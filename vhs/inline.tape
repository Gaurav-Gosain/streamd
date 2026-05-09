# Inline (default) mode. Streams in alt-screen for the live view,
# then prints the final clean output to scrollback on exit.

Output vhs/out/inline.gif

Source vhs/_common.tape
Require streamd
Require jq

Hide
Type "export PS1='\033[38;5;141m>\033[0m '"
Enter
Type "clear"
Enter
Show

Sleep 400ms

Type "vhs/fixtures/stream-sse.sh | streamd"
Sleep 300ms
Enter
Sleep 8s
