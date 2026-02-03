export namespace client_sdk {
	
	export class Member {
	    NodeID: string;
	    Role: string;
	    IsOnline: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Member(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.NodeID = source["NodeID"];
	        this.Role = source["Role"];
	        this.IsOnline = source["IsOnline"];
	    }
	}

}

export namespace storage {
	
	export class ChannelTag {
	    ID: string;
	    ChannelID: string;
	    Name: string;
	    Color: string;
	
	    static createFrom(source: any = {}) {
	        return new ChannelTag(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ChannelID = source["ChannelID"];
	        this.Name = source["Name"];
	        this.Color = source["Color"];
	    }
	}
	export class ChatItem {
	    ID: string;
	    Name: string;
	    Type: string;
	    Avatar: string;
	    // Go type: time
	    LastMessage: any;
	    UnreadCount: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Type = source["Type"];
	        this.Avatar = source["Avatar"];
	        this.LastMessage = this.convertValues(source["LastMessage"], null);
	        this.UnreadCount = source["UnreadCount"];
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
	export class Message {
	    ID: number;
	    MessageID: string;
	    SenderID: string;
	    Target: string;
	    Content: string;
	    // Go type: time
	    Timestamp: any;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.MessageID = source["MessageID"];
	        this.SenderID = source["SenderID"];
	        this.Target = source["Target"];
	        this.Content = source["Content"];
	        this.Timestamp = this.convertValues(source["Timestamp"], null);
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
	export class WikiPage {
	    ID: string;
	    ChannelID: string;
	    Title: string;
	    Content: string;
	    // Go type: time
	    UpdatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new WikiPage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.ChannelID = source["ChannelID"];
	        this.Title = source["Title"];
	        this.Content = source["Content"];
	        this.UpdatedAt = this.convertValues(source["UpdatedAt"], null);
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

