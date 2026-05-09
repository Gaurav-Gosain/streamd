# --info flag. Prints model name, token usage, and speed once the
# response completes.

Output vhs/out/info.gif

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

Type "vhs/fixtures/stream-info.sh | streamd --info"
Sleep 300ms
Enter
Sleep 5s
