export namespace app {
	
	export class Status {
	    root: string;
	    entryCount: number;
	    tagCount: number;
	    scanning: boolean;
	    maxEntries: number;
	    remaining: number;
	    cloudOnly: number;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.root = source["root"];
	        this.entryCount = source["entryCount"];
	        this.tagCount = source["tagCount"];
	        this.scanning = source["scanning"];
	        this.maxEntries = source["maxEntries"];
	        this.remaining = source["remaining"];
	        this.cloudOnly = source["cloudOnly"];
	    }
	}
	export class TagEditResult {
	    path: string;
	    ok: boolean;
	    error?: string;
	    entry?: model.Entry;
	
	    static createFrom(source: any = {}) {
	        return new TagEditResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.ok = source["ok"];
	        this.error = source["error"];
	        this.entry = this.convertValues(source["entry"], model.Entry);
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

export namespace linkcard {
	
	export class Preview {
	    url: string;
	    kind: string;
	    title: string;
	    description?: string;
	    image?: string;
	    siteName?: string;
	    author?: string;
	
	    static createFrom(source: any = {}) {
	        return new Preview(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.url = source["url"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.image = source["image"];
	        this.siteName = source["siteName"];
	        this.author = source["author"];
	    }
	}

}

export namespace model {
	
	export class AudioMeta {
	    title?: string;
	    artist?: string;
	    album?: string;
	    genre?: string;
	    year?: string;
	    durationSec?: number;
	    hasCoverArt?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new AudioMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.artist = source["artist"];
	        this.album = source["album"];
	        this.genre = source["genre"];
	        this.year = source["year"];
	        this.durationSec = source["durationSec"];
	        this.hasCoverArt = source["hasCoverArt"];
	    }
	}
	export class ImageMeta {
	    width?: number;
	    height?: number;
	    taken?: string;
	    make?: string;
	    model?: string;
	    lens?: string;
	
	    static createFrom(source: any = {}) {
	        return new ImageMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	        this.taken = source["taken"];
	        this.make = source["make"];
	        this.model = source["model"];
	        this.lens = source["lens"];
	    }
	}
	export class Entry {
	    path: string;
	    relPath: string;
	    name: string;
	    ext: string;
	    format: string;
	    kind: string;
	    size: number;
	    // Go type: time
	    modTime: any;
	    tags: string[];
	    title: string;
	    preview?: string;
	    links?: string[];
	    tagPage?: string;
	    writable: boolean;
	    image?: ImageMeta;
	    audio?: AudioMeta;
	    err?: string;
	    cloudOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.relPath = source["relPath"];
	        this.name = source["name"];
	        this.ext = source["ext"];
	        this.format = source["format"];
	        this.kind = source["kind"];
	        this.size = source["size"];
	        this.modTime = this.convertValues(source["modTime"], null);
	        this.tags = source["tags"];
	        this.title = source["title"];
	        this.preview = source["preview"];
	        this.links = source["links"];
	        this.tagPage = source["tagPage"];
	        this.writable = source["writable"];
	        this.image = this.convertValues(source["image"], ImageMeta);
	        this.audio = this.convertValues(source["audio"], AudioMeta);
	        this.err = source["err"];
	        this.cloudOnly = source["cloudOnly"];
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

export namespace store {
	
	export class RelatedPage {
	    target?: string;
	    path?: string;
	    title: string;
	    relPath?: string;
	    preview?: string;
	    tags?: string[];
	    kind?: string;
	    cloudOnly?: boolean;
	    tagPage?: string;
	
	    static createFrom(source: any = {}) {
	        return new RelatedPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.path = source["path"];
	        this.title = source["title"];
	        this.relPath = source["relPath"];
	        this.preview = source["preview"];
	        this.tags = source["tags"];
	        this.kind = source["kind"];
	        this.cloudOnly = source["cloudOnly"];
	        this.tagPage = source["tagPage"];
	    }
	}
	export class Related {
	    outgoing: RelatedPage[];
	    incoming: RelatedPage[];
	    sameTag: RelatedPage[];
	    tagged: RelatedPage[];
	    taggedTotal: number;
	    duplicates: RelatedPage[];
	
	    static createFrom(source: any = {}) {
	        return new Related(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.outgoing = this.convertValues(source["outgoing"], RelatedPage);
	        this.incoming = this.convertValues(source["incoming"], RelatedPage);
	        this.sameTag = this.convertValues(source["sameTag"], RelatedPage);
	        this.tagged = this.convertValues(source["tagged"], RelatedPage);
	        this.taggedTotal = source["taggedTotal"];
	        this.duplicates = this.convertValues(source["duplicates"], RelatedPage);
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
	
	export class TagPageGroup {
	    tag: string;
	    pages: model.Entry[];
	
	    static createFrom(source: any = {}) {
	        return new TagPageGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag = source["tag"];
	        this.pages = this.convertValues(source["pages"], model.Entry);
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
	export class Result {
	    entries: model.Entry[];
	    total: number;
	    tagPages: TagPageGroup[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], model.Entry);
	        this.total = source["total"];
	        this.tagPages = this.convertValues(source["tagPages"], TagPageGroup);
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
	export class SearchOptions {
	    query: string;
	    sort: string;
	    offset: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new SearchOptions(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = source["query"];
	        this.sort = source["sort"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	    }
	}
	
	export class TagSuggestion {
	    tag: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new TagSuggestion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag = source["tag"];
	        this.count = source["count"];
	    }
	}

}

