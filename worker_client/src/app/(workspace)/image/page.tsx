"use client";

import { Wand2, Sparkles, Image as ImageIcon } from "lucide-react";
import Link from "next/link";

const tools = [
  {
    id: "icons",
    title: "Icon Generator",
    description: "Create custom icons in batch. Line, filled, duo-tone styles.",
    icon: <Wand2 size={32} strokeWidth={1.5} />,
    href: "/image/icons",
    gradient: "from-violet-500 to-purple-600",
  },
  {
    id: "enhance",
    title: "Image Enhance",
    description: "Upscale, denoise, and enhance your images with AI.",
    icon: <Sparkles size={32} strokeWidth={1.5} />,
    href: "/image/enhance",
    gradient: "from-amber-500 to-orange-600",
  },
  {
    id: "generate",
    title: "Image Generate",
    description: "Generate images from text prompts using AI.",
    icon: <ImageIcon size={32} strokeWidth={1.5} />,
    href: "/image/generate",
    gradient: "from-cyan-500 to-blue-600",
  },
];

export default function ImagePage() {
  return (
    <div className="flex-1 flex flex-col">
      {/* Header */}
      <header className="h-14 flex items-center px-6 border-b border-border-subtle">
        <h1 className="text-lg font-semibold">AI Image</h1>
      </header>

      {/* Tool grid */}
      <div className="flex-1 p-6">
        <div className="max-w-4xl mx-auto">
          <p className="text-text-secondary mb-6">
            Select a tool to get started
          </p>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {tools.map((tool) => (
              <Link
                key={tool.id}
                href={tool.href}
                className="group relative bg-bg-tertiary rounded-xl border border-border-subtle p-6 hover:bg-bg-hover hover:border-border-light transition-all duration-200"
              >
                <div
                  className={`w-14 h-14 rounded-xl bg-gradient-to-br ${tool.gradient} flex items-center justify-center text-white mb-4 group-hover:scale-105 transition-transform`}
                >
                  {tool.icon}
                </div>
                <h3 className="text-lg font-semibold text-text-primary mb-2">
                  {tool.title}
                </h3>
                <p className="text-sm text-text-tertiary leading-relaxed">
                  {tool.description}
                </p>
              </Link>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
