# Thinking / reasoning support. Models like Qwen and DeepSeek expose
# chain-of-thought via a `reasoning_content` field; streamd renders it
# as an italic header + blockquote, separated from the final answer
# by a horizontal rule.

Output vhs/out/thinking.gif

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

Type "vhs/fixtures/stream-thinking.sh | streamd"
Sleep 300ms
Enter
Sleep 6s
