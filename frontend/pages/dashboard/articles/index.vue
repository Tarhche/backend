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
										<tbody v-if="!pending">
											<tr v-for="(article, index) in data.items" :key="index">
											<th scope="row">{{ index + 1 }}</th>
											<td>{{ article.title }}</td>
											<td>{{ article.published_at }}</td>
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
											<tr v-if="data.items.length == 0">
											<td colspan="5">
												<p>هیچ مقاله ای وجود ندارد</p>
											</td>
											</tr>
										</tbody>
										</table>
								</div>
								<nav v-if="!pending && data.pagination.total_pages > 1" aria-label="Page navigation example">
									<ul class="pagination">
										<li class="page-item">
											<a class="page-link" href="#" aria-label="Previous">
												<span aria-hidden="true">&laquo;</span>
											</a>
										</li>
										<li class="page-item"><a class="page-link" href="#">1</a></li>
										<li class="page-item"><a class="page-link" href="#">2</a></li>
										<li class="page-item"><a class="page-link" href="#">3</a></li>
										<li class="page-item">
											<a class="page-link" href="#" aria-label="Next">
												<span aria-hidden="true">&raquo;</span>
											</a>
										</li>
									</ul>
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

const { data, pending, error } = await useAsyncData(
	'dashboard.articles.index',
	useDashboardArticles().index
)

async function deleteArticle(uuid:string) {
	if (!confirm('آیا میخواهید این مقاله را حذف کنید؟')) {
		return
	}

	await useDashboardArticles().delete(uuid)

	data.value.items = data.value.items.filter((article) => article.uuid != uuid)
}
</script>
