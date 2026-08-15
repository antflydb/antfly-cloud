export type MoodValue =
  | 'happy' | 'wholesome' | 'playful' | 'sassy' | 'chaotic'
  | 'shocked' | 'angry' | 'awkward' | 'sad' | 'scared'
  | 'cool' | 'confused' | 'flirty' | 'excited' | 'chill';

export const MOOD_EMOJIS: { value: MoodValue; emoji: string; label: string }[] = [
  { value: 'happy',     emoji: '😊', label: 'Happy' },
  { value: 'wholesome', emoji: '🥰', label: 'Wholesome' },
  { value: 'playful',   emoji: '😜', label: 'Playful' },
  { value: 'sassy',     emoji: '😏', label: 'Sassy' },
  { value: 'chaotic',   emoji: '🤪', label: 'Chaotic' },
  { value: 'shocked',   emoji: '😱', label: 'Shocked' },
  { value: 'angry',     emoji: '😤', label: 'Angry' },
  { value: 'awkward',   emoji: '😬', label: 'Awkward' },
  { value: 'sad',       emoji: '😢', label: 'Sad' },
  { value: 'scared',    emoji: '😨', label: 'Scared' },
  { value: 'cool',      emoji: '😎', label: 'Cool' },
  { value: 'confused',  emoji: '🤔', label: 'Confused' },
  { value: 'flirty',    emoji: '😍', label: 'Flirty' },
  { value: 'excited',   emoji: '🥳', label: 'Excited' },
  { value: 'chill',     emoji: '😴', label: 'Chill' },
];

interface MoodFilterBarProps {
  selected: MoodValue | null;
  onSelect: (mood: MoodValue | null) => void;
}

export function MoodFilterBar({ selected, onSelect }: MoodFilterBarProps) {
  return (
    <div className="flex gap-1 overflow-x-auto scrollbar-hide py-2 -mx-1 px-1">
      {MOOD_EMOJIS.map(({ value, emoji, label }) => (
        <button
          key={value}
          onClick={() => onSelect(selected === value ? null : value)}
          className={`flex flex-col items-center shrink-0 px-2 py-1 rounded-lg transition-colors cursor-pointer ${
            selected === value
              ? 'ring-2 ring-[hsl(var(--ring))] bg-[hsl(var(--accent))]'
              : 'hover:bg-[hsl(var(--muted))]'
          }`}
          title={label}
        >
          <span className="text-xl leading-none">{emoji}</span>
          <span className="text-[10px] mt-0.5 text-[hsl(var(--muted-foreground))] leading-tight">{label}</span>
        </button>
      ))}
    </div>
  );
}
