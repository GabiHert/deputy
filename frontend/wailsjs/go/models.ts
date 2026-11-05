export namespace app {
	
	export class RepoStatsDTO {
	    exists: boolean;
	    files: number;
	    commits: number;
	    lastLearned: number;
	    remote: string;
	
	    static createFrom(source: any = {}) {
	        return new RepoStatsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.files = source["files"];
	        this.commits = source["commits"];
	        this.lastLearned = source["lastLearned"];
	        this.remote = source["remote"];
	    }
	}
	export class AgentDTO {
	    id: string;
	    name: string;
	    enabled: boolean;
	    repoPath: string;
	    instructions: string;
	    chats: string[];
	    mode: string;
	    trigger: string;
	    waitSeconds: number;
	    maxPerHour: number;
	    prefix: string;
	    push: boolean;
	    readImages: boolean;
	    transcribeVoice: boolean;
	    sendImages: boolean;
	    voiceReplies: boolean;
	    web: boolean;
	    bash: boolean;
	    model: string;
	    language: string;
	    stats: RepoStatsDTO;
	
	    static createFrom(source: any = {}) {
	        return new AgentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.repoPath = source["repoPath"];
	        this.instructions = source["instructions"];
	        this.chats = source["chats"];
	        this.mode = source["mode"];
	        this.trigger = source["trigger"];
	        this.waitSeconds = source["waitSeconds"];
	        this.maxPerHour = source["maxPerHour"];
	        this.prefix = source["prefix"];
	        this.push = source["push"];
	        this.readImages = source["readImages"];
	        this.transcribeVoice = source["transcribeVoice"];
	        this.sendImages = source["sendImages"];
	        this.voiceReplies = source["voiceReplies"];
	        this.web = source["web"];
	        this.bash = source["bash"];
	        this.model = source["model"];
	        this.language = source["language"];
	        this.stats = this.convertValues(source["stats"], RepoStatsDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FileDTO {
	    path: string;
	    content: string;
	    summary: string;
	
	    static createFrom(source: any = {}) {
	        return new FileDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.content = source["content"];
	        this.summary = source["summary"];
	    }
	}
	export class ChatRefDTO {
	    jid: string;
	    name: string;
	    isGroup: boolean;
	    members: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatRefDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jid = source["jid"];
	        this.name = source["name"];
	        this.isGroup = source["isGroup"];
	        this.members = source["members"];
	    }
	}
	export class AgentProposalDTO {
	    name: string;
	    chats: ChatRefDTO[];
	    mode: string;
	    trigger: string;
	    maxPerHour: number;
	    readImages: boolean;
	    transcribeVoice: boolean;
	    voiceReplies: boolean;
	    sendImages: boolean;
	    language: string;
	    repoPath: string;
	    instructions: string;
	    knowledge: FileDTO[];
	    createdAgentId: string;
	
	    static createFrom(source: any = {}) {
	        return new AgentProposalDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.chats = this.convertValues(source["chats"], ChatRefDTO);
	        this.mode = source["mode"];
	        this.trigger = source["trigger"];
	        this.maxPerHour = source["maxPerHour"];
	        this.readImages = source["readImages"];
	        this.transcribeVoice = source["transcribeVoice"];
	        this.voiceReplies = source["voiceReplies"];
	        this.sendImages = source["sendImages"];
	        this.language = source["language"];
	        this.repoPath = source["repoPath"];
	        this.instructions = source["instructions"];
	        this.knowledge = this.convertValues(source["knowledge"], FileDTO);
	        this.createdAgentId = source["createdAgentId"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AssistantChatDTO {
	    id: string;
	    title: string;
	    purpose: string;
	    agentId: string;
	    preview: string;
	    createdAt: number;
	    updatedAt: number;
	    busy: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AssistantChatDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.purpose = source["purpose"];
	        this.agentId = source["agentId"];
	        this.preview = source["preview"];
	        this.createdAt = source["createdAt"];
	        this.updatedAt = source["updatedAt"];
	        this.busy = source["busy"];
	    }
	}
	export class SettingChangeDTO {
	    field: string;
	    label: string;
	    from: string;
	    to: string;
	    value: string;
	
	    static createFrom(source: any = {}) {
	        return new SettingChangeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.label = source["label"];
	        this.from = source["from"];
	        this.to = source["to"];
	        this.value = source["value"];
	    }
	}
	export class ChangeProposalDTO {
	    agentId: string;
	    agentName: string;
	    summary: string;
	    oldInstructions: string;
	    instructions: string;
	    knowledge: FileDTO[];
	    settings: SettingChangeDTO[];
	    commit: string;
	
	    static createFrom(source: any = {}) {
	        return new ChangeProposalDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentId = source["agentId"];
	        this.agentName = source["agentName"];
	        this.summary = source["summary"];
	        this.oldInstructions = source["oldInstructions"];
	        this.instructions = source["instructions"];
	        this.knowledge = this.convertValues(source["knowledge"], FileDTO);
	        this.settings = this.convertValues(source["settings"], SettingChangeDTO);
	        this.commit = source["commit"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QuestionDTO {
	    id: string;
	    label: string;
	    kind: string;
	    options: string[];
	    placeholder: string;
	
	    static createFrom(source: any = {}) {
	        return new QuestionDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.options = source["options"];
	        this.placeholder = source["placeholder"];
	    }
	}
	export class FormDTO {
	    title: string;
	    questions: QuestionDTO[];
	
	    static createFrom(source: any = {}) {
	        return new FormDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.questions = this.convertValues(source["questions"], QuestionDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CardDTO {
	    kind: string;
	    status: string;
	    result: string;
	    ask?: FormDTO;
	    answers: Record<string, string>;
	    agent?: AgentProposalDTO;
	    change?: ChangeProposalDTO;
	
	    static createFrom(source: any = {}) {
	        return new CardDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kind = source["kind"];
	        this.status = source["status"];
	        this.result = source["result"];
	        this.ask = this.convertValues(source["ask"], FormDTO);
	        this.answers = source["answers"];
	        this.agent = this.convertValues(source["agent"], AgentProposalDTO);
	        this.change = this.convertValues(source["change"], ChangeProposalDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AssistantMessageDTO {
	    id: string;
	    chatId: string;
	    role: string;
	    text: string;
	    at: number;
	    card?: CardDTO;
	
	    static createFrom(source: any = {}) {
	        return new AssistantMessageDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.chatId = source["chatId"];
	        this.role = source["role"];
	        this.text = source["text"];
	        this.at = source["at"];
	        this.card = this.convertValues(source["card"], CardDTO);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AttachmentDTO {
	    path: string;
	    url: string;
	    bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new AttachmentDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.url = source["url"];
	        this.bytes = source["bytes"];
	    }
	}
	export class QueryResultDTO {
	    query: string;
	    shape: string;
	    items: any;
	
	    static createFrom(source: any = {}) {
	        return new QueryResultDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.shape = source["shape"];
	        this.items = source["items"];
	    }
	}
	export class BoardColumnDTO {
	    key: string;
	    label: string;
	    numeric: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BoardColumnDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.numeric = source["numeric"];
	    }
	}
	export class BoardSourceDTO {
	    query: string;
	    params: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new BoardSourceDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.params = source["params"];
	    }
	}
	export class BoardBlockDTO {
	    id: string;
	    type: string;
	    title: string;
	    span: number;
	    source?: BoardSourceDTO;
	    columns: BoardColumnDTO[];
	    text: string;
	    html: string;
	    js: string;
	    height: number;
	    agentId: string;
	    prompt: string;
	    schedule: string;
	    form?: FormDTO;
	    data?: QueryResultDTO;
	    agent?: AgentDTO;
	    error: string;
	    textAt: number;
	    refreshing: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BoardBlockDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.title = source["title"];
	        this.span = source["span"];
	        this.source = this.convertValues(source["source"], BoardSourceDTO);
	        this.columns = this.convertValues(source["columns"], BoardColumnDTO);
	        this.text = source["text"];
	        this.html = source["html"];
	        this.js = source["js"];
	        this.height = source["height"];
	        this.agentId = source["agentId"];
	        this.prompt = source["prompt"];
	        this.schedule = source["schedule"];
	        this.form = this.convertValues(source["form"], FormDTO);
	        this.data = this.convertValues(source["data"], QueryResultDTO);
	        this.agent = this.convertValues(source["agent"], AgentDTO);
	        this.error = source["error"];
	        this.textAt = source["textAt"];
	        this.refreshing = source["refreshing"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class BoardDTO {
	    blocks: BoardBlockDTO[];
	    layoutAt: number;
	    chatId: string;
	    building: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BoardDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blocks = this.convertValues(source["blocks"], BoardBlockDTO);
	        this.layoutAt = source["layoutAt"];
	        this.chatId = source["chatId"];
	        this.building = source["building"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	export class ChatDTO {
	    jid: string;
	    name: string;
	    isGroup: boolean;
	    members: number;
	    lastText: string;
	    lastAt: number;
	    agentId: string;
	    agentName: string;
	    mode: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jid = source["jid"];
	        this.name = source["name"];
	        this.isGroup = source["isGroup"];
	        this.members = source["members"];
	        this.lastText = source["lastText"];
	        this.lastAt = source["lastAt"];
	        this.agentId = source["agentId"];
	        this.agentName = source["agentName"];
	        this.mode = source["mode"];
	    }
	}
	
	export class ClaudeDTO {
	    found: boolean;
	    path: string;
	    version: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ClaudeDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.found = source["found"];
	        this.path = source["path"];
	        this.version = source["version"];
	        this.error = source["error"];
	    }
	}
	export class DefaultsDTO {
	    mode: string;
	    maxPerHour: number;
	    waitSeconds: number;
	    prefix: string;
	
	    static createFrom(source: any = {}) {
	        return new DefaultsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.maxPerHour = source["maxPerHour"];
	        this.waitSeconds = source["waitSeconds"];
	        this.prefix = source["prefix"];
	    }
	}
	
	
	export class MessageDTO {
	    id: string;
	    at: number;
	    senderName: string;
	    fromMe: boolean;
	    kind: string;
	    text: string;
	    mediaUrl: string;
	    seconds: number;
	    transcript: string;
	    mentionsMe: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MessageDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.senderName = source["senderName"];
	        this.fromMe = source["fromMe"];
	        this.kind = source["kind"];
	        this.text = source["text"];
	        this.mediaUrl = source["mediaUrl"];
	        this.seconds = source["seconds"];
	        this.transcript = source["transcript"];
	        this.mentionsMe = source["mentionsMe"];
	    }
	}
	
	
	
	export class RunDTO {
	    id: string;
	    at: number;
	    agentId: string;
	    agentName: string;
	    chatJid: string;
	    chatName: string;
	    isGroup: boolean;
	    status: string;
	    incoming: MessageDTO[];
	    reply: string;
	    images: AttachmentDTO[];
	    voice: boolean;
	    note: string;
	    commit: string;
	    files: string[];
	    error: string;
	    sentAt: number;
	
	    static createFrom(source: any = {}) {
	        return new RunDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.at = source["at"];
	        this.agentId = source["agentId"];
	        this.agentName = source["agentName"];
	        this.chatJid = source["chatJid"];
	        this.chatName = source["chatName"];
	        this.isGroup = source["isGroup"];
	        this.status = source["status"];
	        this.incoming = this.convertValues(source["incoming"], MessageDTO);
	        this.reply = source["reply"];
	        this.images = this.convertValues(source["images"], AttachmentDTO);
	        this.voice = source["voice"];
	        this.note = source["note"];
	        this.commit = source["commit"];
	        this.files = source["files"];
	        this.error = source["error"];
	        this.sentAt = source["sentAt"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SettingsDTO {
	    defaults: DefaultsDTO;
	    mediaRetentionDays: number;
	    openAtLogin: boolean;
	    notify: boolean;
	    mediaDir: string;
	    mediaBytes: number;
	    mediaFiles: number;
	
	    static createFrom(source: any = {}) {
	        return new SettingsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.defaults = this.convertValues(source["defaults"], DefaultsDTO);
	        this.mediaRetentionDays = source["mediaRetentionDays"];
	        this.openAtLogin = source["openAtLogin"];
	        this.notify = source["notify"];
	        this.mediaDir = source["mediaDir"];
	        this.mediaBytes = source["mediaBytes"];
	        this.mediaFiles = source["mediaFiles"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SpeechModelDTO {
	    language: string;
	    label: string;
	    installed: boolean;
	    sizeBytes: number;
	    downloading: boolean;
	    done: number;
	    total: number;
	    phase: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SpeechModelDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.label = source["label"];
	        this.installed = source["installed"];
	        this.sizeBytes = source["sizeBytes"];
	        this.downloading = source["downloading"];
	        this.done = source["done"];
	        this.total = source["total"];
	        this.phase = source["phase"];
	        this.error = source["error"];
	    }
	}
	export class StatsDTO {
	    sent: number;
	    waiting: number;
	    quiet: number;
	    learned: number;
	    failed: number;
	    photos: number;
	    voice: number;
	    imagesSent: number;
	
	    static createFrom(source: any = {}) {
	        return new StatsDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.sent = source["sent"];
	        this.waiting = source["waiting"];
	        this.quiet = source["quiet"];
	        this.learned = source["learned"];
	        this.failed = source["failed"];
	        this.photos = source["photos"];
	        this.voice = source["voice"];
	        this.imagesSent = source["imagesSent"];
	    }
	}
	export class WhatsAppDTO {
	    state: string;
	    qr: string;
	    qrExpiresAt: number;
	    phone: string;
	    pushName: string;
	    since: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new WhatsAppDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.qr = source["qr"];
	        this.qrExpiresAt = source["qrExpiresAt"];
	        this.phone = source["phone"];
	        this.pushName = source["pushName"];
	        this.since = source["since"];
	        this.error = source["error"];
	    }
	}
	export class StatusDTO {
	    whatsapp: WhatsAppDTO;
	    claude: ClaudeDTO;
	    version: string;
	    waiting: number;
	
	    static createFrom(source: any = {}) {
	        return new StatusDTO(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.whatsapp = this.convertValues(source["whatsapp"], WhatsAppDTO);
	        this.claude = this.convertValues(source["claude"], ClaudeDTO);
	        this.version = source["version"];
	        this.waiting = source["waiting"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

