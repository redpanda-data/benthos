package lsp

type CodeSnippet struct {
	Body        string
	Description string
}

var CodeSnippets = map[string]CodeSnippet{
	"#story": {
		Body:        "no cap frfr ${1:name} was ${2:activity} and suddenly 💀 ${3:unexpected_event} happened ong\neveryone was like sheeeesh 😱 and ${1:name} just said \"${4:reaction}\" frfr",
		Description: "Generate a dramatic story template",
	},
	"#rant": {
		Body:        "NAH FR THO 😤 ${1:topic} is actually WILD 💯 like how are people even ${2:action} fr⁉️ this is actually so ${3:adjective} i cant even- 💀 ${4:extra_thoughts}",
		Description: "Create a passionate rant template",
	},
	"#ratio": {
		Body:        "L + ratio + ${1:insult} + touch grass + ${2:another_insult} + cope + seethe + ${3:final_insult} 💀💀💀",
		Description: "Generate a Twitter-style ratio response",
	},
	"#reaction": {
		Body:        "*${1:name} has entered the chat*\n👁️👄👁️\n\"${2:reaction}\" -🤓\n*${3:action}* ✨\n\"${4:response}\" -🗿",
		Description: "Create a chat-style reaction scene",
	},
	"#flex": {
		Body:        "caught in 4k 📸 absolutely ${1:adjective} frfr no cap in my ${2:item} era ✨ actually built different ${3:emoji} ${4:extra_flex}",
		Description: "Generate a flexing/bragging template",
	},
	"#vibe": {
		Body:        "bestie check ⚠️ vibe status: ${1:status}\ndrip level: ${2:level}\nenergy: ${3:energy_type}\nmood: ${4:current_mood}",
		Description: "Create a vibe check status",
	},
	"#spill": {
		Body:        "YALL 😭 THE TEA IS SCALDING ☕️\n1. ${1:first_gossip} fr fr\n2. ${2:second_gossip} no cap\n3. ${3:third_gossip} ong\nAND THATS ON PERIODT ${4:emoji}",
		Description: "Generate a gossip/tea spilling template",
	},
	"#review": {
		Body:        "review time besties 🤩\n\nthe ${1:thing}: ${2:emoji}\nthe vibe: ${3:vibe_rating}\nthe moment: ${4:moment_rating}\n\nverdict: ${5:final_verdict} no cap frfr",
		Description: "Create a review template",
	},
	"#slay": {
		Body:        "slay count: ${1:number} 💅\nslay type: ${2:type} ✨\nslay energy: ${3:energy} 💃\nslay result: ${4:result} 👑",
		Description: "Generate a slay report",
	},
	"#challenge": {
		Body:        "THE ${1:challenge_name} CHALLENGE 😱\n\nrules:\n1. ${2:first_rule} 😤\n2. ${3:second_rule} 💯\n3. ${4:third_rule} 🔥\n\ndo it for the vine bestie ✨",
		Description: "Create a viral challenge template",
	},
	"#exposed": {
		Body:        "EXPOSED THREAD 📝\n\n${1:name} GOT CAUGHT IN 4K 📸\n\nEVIDENCE:\n1. ${2:evidence1} 👀\n2. ${3:evidence2} 💀\n3. ${4:evidence3} ⚠️",
		Description: "Generate an expose thread template",
	},
	"#pov": {
		Body:        "POV: ${1:situation} 👁️👄👁️\n\nme: ${2:reaction}\nthem: ${3:their_reaction}\neveryone else: ${4:crowd_reaction}",
		Description: "Create a POV scenario",
	},
	"#fit": {
		Body:        "FIT CHECK 👕\n\ntop: ${1:top} ⭐️\nbottom: ${2:bottom} 🔥\nkicks: ${3:shoes} 👟\naccessories: ${4:accessories} ✨\n\ndrip status: ${5:status} 💧",
		Description: "Generate a fit check template",
	},
	"#trend": {
		Body:        "NEW TREND ALERT 🚨\n\nwhat: ${1:trend_name}\nwhy: ${2:reason}\nvibe: ${3:vibe_rating}\ndifficulty: ${4:difficulty}\n\nrating: ${5:rating}/10 would recommend fr fr",
		Description: "Create a trend review template",
	},
	"#argument": {
		Body:        "bestie said: \"${1:their_point}\" 🤡\nme, an intellectual: \"${2:your_point}\" 🧠\nthe facts: ${3:actual_facts} 📝\nthe outcome: ${4:result} 💅",
		Description: "Generate an argument template",
	},
}
