import type { Metadata } from "next";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { MockBanner } from "@/components/mock-banner";
import "./globals.css";

export const metadata: Metadata = {
  title: "WorkLingo",
  description: "Practice Business English in a simulated Slack workspace with AI teammates.",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className="h-full antialiased">
      <body className="h-full min-h-full flex flex-col">
        <TooltipProvider>
          <MockBanner />
          {children}
        </TooltipProvider>
        <Toaster position="bottom-right" richColors closeButton />
      </body>
    </html>
  );
}
