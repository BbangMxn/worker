export type DocumentType = "doc" | "sheet" | "slide" | "pdf" | "image" | "other";

export interface Document {
  id: string;
  title: string;
  type: DocumentType;
  content?: string;
  thumbnail?: string;
  size: number;
  folder?: string;
  starred: boolean;
  shared: boolean;
  sharedWith?: string[];
  owner: string;
  createdAt: string;
  updatedAt: string;
}

export interface DocumentFolder {
  id: string;
  name: string;
  color?: string;
  parentId?: string;
  documentCount: number;
}
