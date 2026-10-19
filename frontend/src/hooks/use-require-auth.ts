"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { setUnauthorizedHandler } from "@/lib/api";
import { useAuth } from "@/store/auth";

/** Redirects to /login when not authenticated. Returns the auth state. */
export function useRequireAuth() {
  const router = useRouter();
  const status = useAuth((s) => s.status);
  const user = useAuth((s) => s.user);
  const init = useAuth((s) => s.init);
  const logout = useAuth((s) => s.logout);

  useEffect(() => {
    void init();
  }, [init]);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      logout();
      router.replace("/login");
    });
    return () => setUnauthorizedHandler(null);
  }, [logout, router]);

  useEffect(() => {
    if (status === "anon") router.replace("/login");
  }, [status, router]);

  return { status, user };
}
