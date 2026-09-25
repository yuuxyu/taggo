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
	
	export class Entry {
	    path: string;
	    relPath: string;
	    name: string;
	    ext: string;
	    size: number;
	    // Go type: time
	    modTime: any;
	    tags: string[];
	    title: string;
	    preview?: string;
	    thumbnail?: string;
	    links?: string[];
	    err?: string;
	    cloudOnly?: boolean;
	    missing?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Entry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.relPath = source["relPath"];
	        this.name = source["name"];
	        this.ext = source["ext"];
	        this.size = source["size"];
	        this.modTime = this.convertValues(source["modTime"], null);
	        this.tags = source["tags"];
	        this.title = source["title"];
	        this.preview = source["preview"];
	        this.thumbnail = source["thumbnail"];
	        this.links = source["links"];
	        this.err = source["err"];
	        this.cloudOnly = source["cloudOnly"];
	        this.missing = source["missing"];
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

export namespace settings {
	
	export class Settings {
	    version: number;
	    startupMode: string;
	    startupFolder?: string;
	    lastFolder?: string;
	    sort: string;
	    scanLimit: number;
	    theme: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.startupMode = source["startupMode"];
	        this.startupFolder = source["startupFolder"];
	        this.lastFolder = source["lastFolder"];
	        this.sort = source["sort"];
	        this.scanLimit = source["scanLimit"];
	        this.theme = source["theme"];
	    }
	}

}

export namespace store {
	
	export class RelatedPage {
	    target?: string;
	    tag?: string;
	    path?: string;
	    title: string;
	    relPath?: string;
	    preview?: string;
	    thumbnail?: string;
	    cloudOnly?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RelatedPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.target = source["target"];
	        this.tag = source["tag"];
	        this.path = source["path"];
	        this.title = source["title"];
	        this.relPath = source["relPath"];
	        this.preview = source["preview"];
	        this.thumbnail = source["thumbnail"];
	        this.cloudOnly = source["cloudOnly"];
	    }
	}
	export class RelatedGroup {
	    tag?: string;
	    page?: RelatedPage;
	    pages: RelatedPage[];
	    more: number;
	
	    static createFrom(source: any = {}) {
	        return new RelatedGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tag = source["tag"];
	        this.page = this.convertValues(source["page"], RelatedPage);
	        this.pages = this.convertValues(source["pages"], RelatedPage);
	        this.more = source["more"];
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
	export class Related {
	    groups: RelatedGroup[];
	    missingLinks: string[];
	    missingTags: string[];
	
	    static createFrom(source: any = {}) {
	        return new Related(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], RelatedGroup);
	        this.missingLinks = source["missingLinks"];
	        this.missingTags = source["missingTags"];
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
	    head: number;
	    pins: string[];
	
	    static createFrom(source: any = {}) {
	        return new Result(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], model.Entry);
	        this.total = source["total"];
	        this.head = source["head"];
	        this.pins = source["pins"];
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

}

