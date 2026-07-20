import { writable } from 'svelte/store';

export type ToastTone = 'success' | 'error' | 'info';

export type Toast = {
  id: number;
  tone: ToastTone;
  title: string;
  detail?: string;
};

let counter = 0;
const MAX_TOASTS = 5;

export const toasts = writable<Toast[]>([]);

export function pushToast(tone: ToastTone, title: string, detail?: string, ttl?: number) {
  if (ttl === undefined) ttl = tone === 'error' ? 7000 : 4200;
  const id = ++counter;
  toasts.update((items) => {
    const next = [...items, { id, tone, title, detail }];
    return next.length > MAX_TOASTS ? next.slice(next.length - MAX_TOASTS) : next;
  });
  if (ttl > 0) {
    setTimeout(() => dismissToast(id), ttl);
  }
  return id;
}

export function dismissToast(id: number) {
  toasts.update((items) => items.filter((toast) => toast.id !== id));
}

export const toast = {
  success: (title: string, detail?: string) => pushToast('success', title, detail),
  error: (title: string, detail?: string) => pushToast('error', title, detail),
  info: (title: string, detail?: string) => pushToast('info', title, detail)
};
