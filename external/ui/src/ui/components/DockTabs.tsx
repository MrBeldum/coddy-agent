import { useT } from "../i18n/I18nProvider";

/**
 * The tab strip of the shared right dock: Tasks and Changes are two faces of
 * one panel rather than two panels competing for the same edge. `tab` names the
 * face on show; `onTab` asks for the other one.
 */
export function DockTabs(props: {
  tab: "tasks" | "changes";
  onTab: (tab: "tasks" | "changes") => void;
}) {
  const { t } = useT();
  const tabs = [
    { id: "tasks" as const, label: t("tasks.panelTitle") },
    { id: "changes" as const, label: t("changes.panelTitle") },
  ];
  return (
    <span className="dock-tabs" role="tablist" aria-label={t("changes.dockTabs")}>
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          role="tab"
          aria-selected={props.tab === tab.id}
          className={"dock-tab" + (props.tab === tab.id ? " is-active" : "")}
          data-testid={`dock-tab-${tab.id}`}
          onClick={() => props.onTab(tab.id)}
        >
          {tab.label}
        </button>
      ))}
    </span>
  );
}
