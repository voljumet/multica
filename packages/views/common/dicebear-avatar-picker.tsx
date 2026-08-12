"use client";

import { useCallback, useState } from "react";
import { Check, Dices } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";

const STYLES = [
  { value: "adventurer", label: "Adventurer" },
  { value: "adventurer-neutral", label: "Adventurer Neutral" },
  { value: "big-ears", label: "Big Ears" },
  { value: "croodles", label: "Croodles" },
  { value: "fun-emoji", label: "Fun Emoji" },
  { value: "lorelei", label: "Lorelei" },
  { value: "micah", label: "Micah" },
  { value: "notionists", label: "Notionists" },
  { value: "open-peeps", label: "Open Peeps" },
  { value: "personas", label: "Personas" },
  { value: "pixel-art", label: "Pixel Art" },
  { value: "bottts-neutral", label: "Robots" },
  { value: "shapes", label: "Shapes" },
  { value: "thumbs", label: "Thumbs" },
] as const;

type StyleValue = (typeof STYLES)[number]["value"];

function dicebearUrl(style: StyleValue, seed: string) {
  return `https://api.dicebear.com/9.x/${style}/svg?seed=${encodeURIComponent(seed)}`;
}

function randomSeed() {
  return Math.random().toString(36).slice(2, 10);
}

interface DicebearAvatarPickerProps {
  onSelect: (url: string) => void;
  trigger: React.ReactElement;
}

export function DicebearAvatarPicker({
  onSelect,
  trigger,
}: DicebearAvatarPickerProps) {
  const [open, setOpen] = useState(false);
  const [style, setStyle] = useState<StyleValue>("adventurer");
  const [seed, setSeed] = useState(randomSeed);

  const url = dicebearUrl(style, seed);

  const handleSelect = useCallback(() => {
    onSelect(url);
    setOpen(false);
  }, [url, onSelect]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={trigger} />
      <PopoverContent className="w-56 space-y-3 p-3" align="start">
        <Select
          value={style}
          onValueChange={(v) => {
            setStyle(v as StyleValue);
            setSeed(randomSeed());
          }}
          items={STYLES}
        >
          <SelectTrigger size="sm" className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STYLES.map((s) => (
              <SelectItem key={s.value} value={s.value}>
                {s.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <div className="flex justify-center">
          <img
            src={url}
            alt="avatar preview"
            className="h-24 w-24 rounded-full bg-muted"
          />
        </div>

        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="flex-1 gap-1 text-xs"
            onClick={() => setSeed(randomSeed())}
          >
            <Dices className="h-3.5 w-3.5" />
            Roll
          </Button>
          <Button
            type="button"
            size="sm"
            className="flex-1 gap-1 text-xs"
            onClick={handleSelect}
          >
            <Check className="h-3.5 w-3.5" />
            Use this
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
