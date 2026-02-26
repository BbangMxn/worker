// API Types - Based on worker backend models
// Email/Calendar/Contact types are defined in entities/ as the single source of truth.
// This file contains API-specific types only.

// ============ Auth ============
export interface User {
  id: string;
  email: string;
  name?: string;
  avatar?: string;
  jobType?: string;
  createdAt: string;
  updatedAt: string;
}

export type OAuthProvider = "gmail" | "outlook";

export interface OAuthConnection {
  id: number;
  userId: string;
  provider: OAuthProvider;
  email: string;
  isConnected: boolean;
  expiresAt: string;
}

// ============ Contact ============
export interface Company {
  id: number;
  userId: string;
  name: string;
  email?: string;
  phone?: string;
  createdAt: string;
  updatedAt: string;
}

export interface ApiContact {
  id: number;
  userId: string;
  companyId?: number;
  company?: Company;
  name: string;
  email: string;
  position?: string;
  phone?: string;
}

// ============ Email ============
export interface EmailAccount {
  id: number;
  email: string;
  provider: OAuthProvider;
  isConnected: boolean;
  lastSyncAt?: string;
}

// ============ Common ============
export interface Macro {
  id: number;
  userId: string;
  name: string;
  shortcut: string;
  content: string;
  category?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Template {
  id: number;
  userId: string;
  name: string;
  subject: string;
  body: string;
  variables: string[];
  category?: string;
  createdAt: string;
  updatedAt: string;
}

// ============ Search ============
export interface SearchResult {
  type: "company" | "contact" | "email";
  id: number;
  title: string;
  subtitle?: string;
  highlight?: string;
}
