import {Popover, ActionIcon, Text} from "@mantine/core";
import {IconInfoCircle} from "@tabler/icons-react";

export function OrphanCommentIndicator() {
  return (
    <Popover position="bottom" shadow="md" withArrow>
      <Popover.Target>
        <ActionIcon color="yellow" variant="transparent">
          <IconInfoCircle style={{width: "70%", height: "70%"}} stroke={1.5} />
        </ActionIcon>
      </Popover.Target>
      <Popover.Dropdown>
        <Text size="sm">والد این کامنت حذف یا مخفی شده است.</Text>
      </Popover.Dropdown>
    </Popover>
  );
}
