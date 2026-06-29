import { writable } from 'svelte/store';

export type ToastTone = 'success' | 'error' | 'info';

export type Toast = {
  id: number;
  tone: ToastTone;
  title: string;
  detail?: string;
};

let counter = 0;

export const toasts = writable<Toast[]>([]);

export function pushToast(tone: ToastTone, title: string, detail?: string, ttl = 4200) {
  const id = ++counter;
  toasts.update((items) => [...items, { id, tone, title, detail }]);
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
