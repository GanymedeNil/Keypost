'use client';

import { useState, useSyncExternalStore } from 'react';
import { adminApi } from '@/lib/api';
import type { VariantFile } from '@/lib/types';

export type UploadState = 'queued' | 'uploading' | 'done' | 'error' | 'cancelled';

export interface UploadOptions {
  version: string;
  status: 'published' | 'draft';
}

export interface UploadItem {
  id: number;
  file: File;
  options: UploadOptions;
  state: UploadState;
  /** 0–100 (bytes sent). */
  progress: number;
  error?: unknown;
}

const CONCURRENCY = 2;

function baseName(filename: string): string {
  const dot = filename.lastIndexOf('.');
  return dot > 0 ? filename.slice(0, dot) : filename;
}

/**
 * Upload queue for one variant: up to 2 parallel multipart uploads with per-file progress,
 * cancel and retry. Framework-agnostic; consumed with useSyncExternalStore (see useUploadQueue).
 */
export class UploadQueue {
  private items: UploadItem[] = [];
  private listeners = new Set<() => void>();
  private controllers = new Map<number, AbortController>();
  private callbacks = new Map<number, () => void>();
  private active = 0;
  private seq = 0;

  constructor(private readonly variantId: number) {}

  subscribe = (listener: () => void) => {
    this.listeners.add(listener);
    return () => {
      this.listeners.delete(listener);
    };
  };

  getSnapshot = () => this.items;

  private set(items: UploadItem[]) {
    this.items = items;
    this.listeners.forEach((l) => l());
  }

  private patch(id: number, patch: Partial<UploadItem>) {
    this.set(this.items.map((i) => (i.id === id ? { ...i, ...patch } : i)));
  }

  /** Queue files; `onUploaded` runs after each successful upload (e.g. to refresh the file list). */
  add(files: File[], options: UploadOptions, onUploaded: () => void) {
    const added = files.map<UploadItem>((file) => ({ id: ++this.seq, file, options, state: 'queued', progress: 0 }));
    added.forEach((i) => this.callbacks.set(i.id, onUploaded));
    this.set([...this.items, ...added]);
    this.pump();
  }

  retry(id: number) {
    this.patch(id, { state: 'queued', progress: 0, error: undefined });
    this.pump();
  }

  cancel(id: number) {
    const controller = this.controllers.get(id);
    if (controller) controller.abort();
    else this.patch(id, { state: 'cancelled' });
  }

  remove(id: number) {
    this.callbacks.delete(id);
    this.set(this.items.filter((i) => i.id !== id));
  }

  clearFinished() {
    this.set(this.items.filter((i) => i.state === 'queued' || i.state === 'uploading'));
  }

  private pump() {
    while (this.active < CONCURRENCY) {
      const next = this.items.find((i) => i.state === 'queued');
      if (!next) return;
      this.active++;
      void this.start(next);
    }
  }

  private async start(item: UploadItem) {
    const controller = new AbortController();
    this.controllers.set(item.id, controller);
    this.patch(item.id, { state: 'uploading', progress: 0 });
    const form = new FormData();
    form.append('file', item.file);
    form.append('name', baseName(item.file.name));
    if (item.options.version.trim()) form.append('version', item.options.version.trim());
    form.append('status', item.options.status);
    try {
      await adminApi.upload<VariantFile>(`/variants/${this.variantId}/files`, form, {
        signal: controller.signal,
        onProgress: (loaded, total) => this.patch(item.id, { progress: Math.round((loaded / total) * 100) }),
      });
      this.patch(item.id, { state: 'done', progress: 100 });
      this.callbacks.get(item.id)?.();
    } catch (error) {
      if (error instanceof DOMException && error.name === 'AbortError') this.patch(item.id, { state: 'cancelled' });
      else this.patch(item.id, { state: 'error', error });
    } finally {
      this.controllers.delete(item.id);
      this.active--;
      this.pump();
    }
  }
}

export function useUploadQueue(variantId: number) {
  const [queue] = useState(() => new UploadQueue(variantId));
  const items = useSyncExternalStore(queue.subscribe, queue.getSnapshot, queue.getSnapshot);
  return { queue, items };
}
