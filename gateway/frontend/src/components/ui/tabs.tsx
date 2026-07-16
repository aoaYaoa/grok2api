import * as TabsPrimitive from "@radix-ui/react-tabs";
import type * as React from "react";

import { cn } from "@/shared/lib/cn";

export function Tabs({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Root>) {
  return <TabsPrimitive.Root className={cn("flex flex-col", className)} {...props} />;
}

export function TabsList({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.List>) {
  return <TabsPrimitive.List className={cn("inline-flex h-9 max-w-full isolate items-stretch gap-0.5 overflow-x-auto overflow-y-hidden rounded-md border bg-muted p-[3px] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden", className)} {...props} />;
}

export function TabsTrigger({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Trigger>) {
  return (
    <TabsPrimitive.Trigger
      className={cn("inline-flex h-full min-h-0 min-w-0 shrink-0 items-center justify-center whitespace-nowrap rounded-[4px] border border-transparent px-3 text-xs font-medium leading-none text-muted-foreground outline-none transition-[background-color,border-color,color] hover:text-foreground focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring/50 data-[state=active]:border-border data-[state=active]:bg-background data-[state=active]:text-foreground", className)}
      {...props}
    />
  );
}

export function TabsContent({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return <TabsPrimitive.Content className={cn("min-w-0 outline-none", className)} {...props} />;
}
