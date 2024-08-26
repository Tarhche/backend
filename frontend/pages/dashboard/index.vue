<template>

  <div class="container">
    <div class="row">
      <dashboardSidebar class="col-md-3 ml-sm-auto"/>
      <main class="col-md-9 ml-sm-auto">
        <div class="row">
          <div v-if="hasPermission('articles.index')" class="col-12 mb-4">
            <dashboard-latest-articles />
          </div>
          <div v-else v-if="hasPermission('self.comments.index')" class="col-12 mb-4">
            <dashboard-my-latest-comments />
          </div>
        </div>
      </main>
    </div>
  </div>
</template>

<script lang="ts" setup>
definePageMeta({
  layout: 'dashboard',
})

const permissions = await useUser().permissions()

function hasPermission(permission) {
  return permissions.includes(permission)
}
</script>