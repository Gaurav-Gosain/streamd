# Alt-screen mode. Interactive viewport with scroll bar and status
# line. The stream auto-scrolls to follow new content; once it
# finishes we demonstrate scrolling back through the response.

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

# Watch the viewport auto-scroll while content streams in.
Sleep 7s

# Streaming is done. Now scroll up to show earlier content.
Type "g"
Sleep 1s
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 800ms
Type "G"
Sleep 1s
Type "q"
Sleep 400ms
