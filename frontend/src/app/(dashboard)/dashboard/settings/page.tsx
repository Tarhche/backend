import {Metadata} from "next";
import {Stack, Paper} from "@mantine/core";
import {DashboardBreadcrumbs} from "@/features/breadcrumbs/components/breadcrumbs";
import {AppSettingForm} from "@/features/settings/components/app-setting-form";
import {fetchConfigs} from "@/dal/private/config";
import {withPermissions} from "@/components/with-authorization";

const PAGE_TITLE = "تنظیمات";

export const metadata: Metadata = {
  title: PAGE_TITLE,
};

async function SettingsPage() {
  const config = await fetchConfigs();

  return (
    <Stack>
      <DashboardBreadcrumbs
        crumbs={[
          {
            label: PAGE_TITLE,
          },
        ]}
      />
      <Paper p="lg" withBorder>
        <AppSettingForm
          config={{
            userDefaultRoles: config.user_default_roles.join(""),
          }}
        />
      </Paper>
    </Stack>
  );
}

export default withPermissions(SettingsPage, {
  requiredPermissions: ["config.update"],
});
