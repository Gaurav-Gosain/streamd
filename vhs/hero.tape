# Hero tape. The first thing a README visitor sees.
# Pitch in ~15 seconds: pipe an LLM stream into streamd, watch it
# render styled markdown live as tokens arrive.
#
# Regenerate: `vhs vhs/hero.tape`   (outputs vhs/out/hero.gif)

Output vhs/out/hero.gif

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

# A streaming markdown response rendered live.
Type "vhs/fixtures/stream-sse.sh | streamd"
Sleep 400ms
Enter
Sleep 8s
