interface TabContentProps {
  children: React.ReactNode;
  className?: string;
}

export default function TabContent({ children, className = '' }: TabContentProps) {
  return (
    <div className={`flex-1 min-h-0 overflow-hidden ${className}`}>
      {children}
    </div>
  );
}
