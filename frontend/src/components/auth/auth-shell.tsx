import Link from "next/link";
import { Logo } from "@/components/logo";

export function AuthShell({
  title,
  subtitle,
  children,
  footer,
}: {
  title: string;
  subtitle: string;
  children: React.ReactNode;
  footer: React.ReactNode;
}) {
  return (
    <main className="flex flex-1 flex-col items-center bg-white px-4 py-12">
      <Link href="/" className="mb-10 flex items-center gap-2">
        <Logo className="size-9" />
        <span className="text-2xl font-black tracking-tight text-[#1d1c1d]">WorkLingo</span>
      </Link>
      <h1 className="text-center text-4xl font-bold tracking-tight text-[#1d1c1d]">{title}</h1>
      <p className="mt-3 mb-8 text-center text-lg text-[#454245]">{subtitle}</p>
      <div className="w-full max-w-[400px]">{children}</div>
      <div className="mt-6 text-sm text-[#616061]">{footer}</div>
    </main>
  );
}
