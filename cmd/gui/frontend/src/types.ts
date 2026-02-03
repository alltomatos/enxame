export interface ChannelTag {
    ID: string;
    ChannelID: string;
    Name: string;
    Color: string;
}

export interface MessageTag {
    MessageID: string;
    TagID: string;
    TagName: string;
    TagColor: string;
    TaggedBy: string;
}

export interface Message {
    id: string;
    sender: string;
    content: string;
    channel: string;
    timestamp: number;
    mine: boolean;
    pending?: boolean;
    tags?: MessageTag[]; // New field
}
