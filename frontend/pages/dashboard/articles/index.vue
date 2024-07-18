<template>
	<div class="container">
		<div class="row">
			<dashboardSidebar class="col-md-3 ml-sm-auto"/>
			<main class="col-md-9 ml-sm-auto">

				<nav aria-label="breadcrumb">
					<ol class="breadcrumb">
						<li class="breadcrumb-item">
							<NuxtLink to="/dashboard">داشبورد</NuxtLink>
						</li>
						<li class="breadcrumb-item active" aria-current="page">مقاله ها</li>
					</ol>
				</nav>

				<div class="row">
					<div class="col-12 mb-4 mb-lg-0">
						<div class="card">
							<div class="card-header d-flex justify-content-between">
								<h4>مقاله ها</h4>
								<NuxtLink class="btn btn-primary" to="/dashboard/articles/create">مقاله جدید</NuxtLink>
							</div>
							<div class="card-body">
								<div class="table-responsive">
									<table class="table table-striped table-borderless table-hover align-middle">
										<thead class="border-bottom">
											<tr>
											<th scope="col">#</th>
											<th scope="col">عنوان</th>
											<th scope="col">تاریخ انتشار</th>
											<th scope="col">#</th>
											</tr>
										</thead>
										<tbody v-if="!params.pending">
											<tr v-for="(article, index) in params.data.items" :key="index">
												<th scope="row">{{ index + 1 }}</th>
												<td>{{ article.title }}</td>
												<td>
													<span v-if="useTime().isZeroDate(article.published_at)" class="fa fa-times text-danger"></span>
													<span v-else>{{ useTime().toAgo(article.published_at) }}</span>
												</td>
												<td>
													<NuxtLink :to="`/articles/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
														<span class="fa fa-eye"></span>
													</NuxtLink>
													<NuxtLink :to="`/dashboard/articles/edit/${article.uuid}`" class="btn mx-1 btn-sm btn-primary">
														<span class="fa fa-pen"></span>
													</NuxtLink>
													<button @click.prevent="deleteArticle(article.uuid)" type="button" class="btn mx-1 btn-sm btn-danger">
														<span class="fa fa-trash"></span>
													</button>
												</td>
											</tr>
											<tr v-if="params.data.items.length == 0">
												<td colspan="5">
													<p>هیچ مقاله ای وجود ندارد</p>
												</td>
											</tr>
										</tbody>
									</table>
								</div>
								<nav v-if="!params.pending" aria-label="Page navigation example">
									<Pagination @paginate="load" :current="params.data.pagination.current_page" :pages="params.data.pagination.total_pages" />
								</nav>
							</div>
						</div>
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

useHead({
	title: "مقاله ها"
})

const params = reactive({
	data: [],
	pending: true,
	error: null,
})

await load((useRoute().query.page) || 1)

async function load(page:number) {
	const { data, pending, error } = await useAsyncData(
		'dashboard.articles.index',
		() => useDashboardArticles().index(page)
	)

	params.data = data
	params.pending = pending
	params.error = error
}

async function deleteArticle(uuid:string) {
	if (!confirm('آیا میخواهید این مقاله را حذف کنید؟')) {
		return
	}

	await useDashboardArticles().delete(uuid)

	data.value.items = data.value.items.filter((article) => article.uuid != uuid)
}
</script>
