"use client";

import { create } from "zustand";
import { api, getToken, setToken } from "@/lib/api";
import type { User } from "@/lib/types";

type AuthState = {
  user: User | null;
  token: string | null;
  status: "unknown" | "loading" | "authed" | "anon";
  init: () => Promise<void>;
  login: (email: string, password: string) => Promise<void>;
  register: (username: string, email: string, password: string) => Promise<void>;
  logout: () => void;
};

export const useAuth = create<AuthState>((set, get) => ({
  user: null,
  token: null,
  status: "unknown",
  init: async () => {
    if (get().status === "loading" || get().status === "authed") return;
    const token = getToken();
    if (!token) {
      set({ status: "anon", user: null, token: null });
      return;
    }
    set({ status: "loading", token });
    try {
      const user = await api.me();
      set({ user, status: "authed" });
    } catch {
      setToken(null);
      set({ user: null, token: null, status: "anon" });
    }
  },
  login: async (email, password) => {
    const { token, user } = await api.login(email, password);
    setToken(token);
    set({ token, user, status: "authed" });
  },
  register: async (username, email, password) => {
    const { token, user } = await api.register(username, email, password);
    setToken(token);
    set({ token, user, status: "authed" });
  },
  logout: () => {
    setToken(null);
    set({ token: null, user: null, status: "anon" });
  },
}));
