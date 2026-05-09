# Alt-screen mode. Interactive viewport with scrolling and an
# auto-scrolling status bar.

Output vhs/out/alt.gif

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

Type "vhs/fixtures/stream-long.sh | streamd --alt"
Sleep 300ms
Enter

# Watch the viewport auto-scroll as content streams in.
Sleep 6s

# Scroll up to read earlier content, then back to bottom.
Type "k"
Sleep 200ms
Type "k"
Sleep 200ms
Type "k"
Sleep 800ms
Type "G"
Sleep 800ms
Type "q"
Sleep 500ms
